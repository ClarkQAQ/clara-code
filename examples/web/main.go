package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"clara-code/examples/internal/demo"
	"clara-code/examples/internal/host"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/models"
	"clara-code/pkg/tools"
)

type web struct {
	ctx    context.Context
	config host.Config
	mu     sync.Mutex
	rooms  map[string]*room
}

type room struct {
	*host.Conversation
	questions host.Questions
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (err error) {
	address := flag.String("listen", "127.0.0.1:8090", "HTTP address")
	root := flag.String("root", ".", "workspace directory")
	store := flag.String("store", "", "directory for persistent sessions")
	demoMode := flag.Bool("demo", false, "use a deterministic model")
	budget := flag.Int("context-bytes", 0, "serialized context budget for optional compaction")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	config, err := host.Local(*root)
	if err != nil {
		return err
	}
	if *demoMode {
		config.Model = demo.Model{}
	} else {
		config.Model, config.SelectModel, err = host.ModelFromEnv(ctx)
		if err != nil {
			return err
		}
	}
	if *store != "" {
		config.Store, err = host.FileStore(ctx, *store)
		if err != nil {
			return err
		}
	} else {
		config.Store = core.NewMemoryStore()
	}
	config.Compact.BudgetBytes = *budget
	app := web{ctx: ctx, config: config, rooms: map[string]*room{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", page)
	mux.HandleFunc("POST /sessions", app.open)
	mux.HandleFunc("POST /sessions/{id}/action", app.action)
	mux.HandleFunc("GET /sessions/{id}/events", app.events)
	server := &http.Server{Addr: *address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Print(err)
		}
	}()
	defer func() {
		for _, item := range app.rooms {
			err = errors.Join(err, item.Close(context.Background()))
		}
	}()
	log.Printf("Clara Core Web on http://%s", *address)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (app *web) open(w http.ResponseWriter, r *http.Request) {
	var input struct{ ID string }
	if !decode(w, r, &input) {
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.rooms[input.ID] != nil {
		write(w, map[string]string{"id": input.ID})
		return
	}
	if len(app.rooms) >= 128 {
		http.Error(w, "too many open sessions", http.StatusServiceUnavailable)
		return
	}
	config := app.config
	item := &room{}
	item.questions.Publish = func(ctx context.Context, questions []tools.Question) error {
		return item.Session.Emit(ctx, "input.requested", questions)
	}
	config.Ask = item.questions.Ask
	var err error
	item.Conversation, err = host.Open(r.Context(), config, input.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	app.rooms[item.Session.ID()] = item
	write(w, map[string]string{"id": item.Session.ID()})
}

func (app *web) action(w http.ResponseWriter, r *http.Request) {
	item := app.lookup(r.PathValue("id"))
	if item == nil {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Action, Text string
		Plan         bool
		Answers      map[string]string
	}
	if !decode(w, r, &input) {
		return
	}
	mode := prompt.Execute
	if input.Plan {
		mode = prompt.Plan
	}
	var err error
	switch input.Action {
	case "message", "follow":
		if active := item.Session.Active(); active != nil {
			if input.Action == "follow" {
				err = active.FollowUp(models.NewUserMessage(input.Text))
			} else {
				err = active.Steer(models.NewUserMessage(input.Text))
			}
		} else {
			_, err = item.Start(app.ctx, mode, input.Text)
		}
	case "cancel":
		if active := item.Session.Active(); active != nil {
			active.Cancel()
		}
	case "answer":
		err = item.questions.Answer(input.Answers)
	case "goal", "resume":
		if mode == prompt.Plan || item.Session.Active() != nil {
			err = errors.New("start or resume a goal in Agent mode when the session is idle")
		} else {
			if input.Action == "goal" {
				err = item.Goal.Start(input.Text, 100000, 20)
			} else {
				err = item.Goal.Resume()
			}
			if err == nil {
				_, err = item.Start(app.ctx, mode, "Work on the explicitly requested goal: "+item.Goal.State().Objective)
			}
		}
	case "pause":
		err = item.Goal.Pause()
		if active := item.Session.Active(); err == nil && active != nil {
			active.Cancel()
		}
	case "close":
		err = item.Close(r.Context())
		app.mu.Lock()
		delete(app.rooms, r.PathValue("id"))
		app.mu.Unlock()
	default:
		err = errors.New("unknown action")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	write(w, map[string]bool{"ok": true})
}

func (app *web) events(w http.ResponseWriter, r *http.Request) {
	item := app.lookup(r.PathValue("id"))
	if item == nil {
		http.NotFound(w, r)
		return
	}
	var after uint64
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = r.URL.Query().Get("after")
	}
	if cursor != "" {
		if _, err := fmt.Sscan(cursor, &after); err != nil {
			http.Error(w, "invalid event cursor", http.StatusBadRequest)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(app.ctx, cancel)
	defer stop()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	control := http.NewResponseController(w)
	if err := control.Flush(); err != nil {
		return
	}
	for {
		event, err := item.Session.NextEvent(ctx, after)
		if err != nil {
			if ctx.Err() == nil {
				raw, _ := json.Marshal(err.Error())
				fmt.Fprintf(w, "event: replay-error\ndata: %s\n\n", raw)
				_ = control.Flush()
			}
			return
		}
		raw, err := json.Marshal(event)
		if err != nil {
			return
		}
		after = event.Sequence
		_ = control.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", after, raw); err != nil || control.Flush() != nil {
			return
		}
	}
}

func (app *web) lookup(id string) *room {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.rooms[id]
}

func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		http.Error(w, "send application/json", http.StatusUnsupportedMediaType)
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		source, err := url.Parse(origin)
		if err != nil || source.Host != r.Host {
			http.Error(w, "origin does not match this host", http.StatusForbidden)
			return false
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func write(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
