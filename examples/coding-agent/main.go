package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"clara-code/examples/internal/demo"
	"clara-code/examples/internal/host"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/models"
	"clara-code/pkg/tools"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (err error) {
	root := flag.String("root", ".", "workspace directory")
	demoMode := flag.Bool("demo", false, "use the deterministic demo model")
	store := flag.String("store", "", "directory for persistent sessions")
	resume := flag.String("resume", "", "resume a saved session ID")
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
	}
	config.Compact.BudgetBytes = *budget
	var conversation *host.Conversation
	questions := host.Questions{Publish: func(ctx context.Context, qs []tools.Question) error {
		return conversation.Session.Emit(ctx, "input.requested", qs)
	}}
	config.Ask = questions.Ask
	conversation, err = host.Open(ctx, config, *resume)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = errors.Join(err, conversation.Close(cleanup))
	}()

	fmt.Printf("Session %s\n", conversation.Session.ID())
	fmt.Println("Commands /plan /agent /cancel /follow TEXT /answer JSON /goal TEXT /pause /resume /wait /exit")
	eventsCtx, cancelEvents := context.WithCancel(ctx)
	defer cancelEvents()
	printed := make(chan string, 1)
	go display(eventsCtx, conversation.Session, printed)
	mode := prompt.Execute
	lines := make(chan string)
	scanErrors := make(chan error, 1)
	go scan(ctx, lines, scanErrors)

	var last *core.Run
	wait := func() error {
		if last == nil {
			return nil
		}
		_, err := last.Wait(ctx)
		for {
			select {
			case id, ok := <-printed:
				if ok && id != last.ID {
					continue
				}
			case <-eventsCtx.Done():
			}
			break
		}
		last = nil
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return errors.Join(wait(), <-scanErrors)
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			command, argument, _ := strings.Cut(line, " ")
			var commandErr error
			switch command {
			case "/exit":
				return nil
			case "/wait":
				commandErr = wait()
			case "/plan":
				mode = prompt.Plan
			case "/agent":
				mode = prompt.Execute
			case "/cancel":
				if active := conversation.Session.Active(); active != nil {
					active.Cancel()
				}
			case "/follow":
				if active := conversation.Session.Active(); active != nil {
					commandErr = active.FollowUp(models.NewUserMessage(argument))
				} else {
					last, commandErr = conversation.Start(ctx, mode, argument)
				}
			case "/answer":
				var answers map[string]string
				if commandErr = json.Unmarshal([]byte(argument), &answers); commandErr == nil {
					commandErr = questions.Answer(answers)
				}
			case "/goal":
				if mode == prompt.Plan || conversation.Session.Active() != nil {
					commandErr = errors.New("start a goal in Agent mode when the session is idle")
				} else if commandErr = conversation.Goal.Start(argument, 100000, 20); commandErr == nil {
					last, commandErr = conversation.Start(ctx, mode, "Begin the explicitly requested goal: "+argument)
				}
			case "/pause":
				commandErr = conversation.Goal.Pause()
				if active := conversation.Session.Active(); commandErr == nil && active != nil {
					active.Cancel()
				}
			case "/resume":
				if mode == prompt.Plan || conversation.Session.Active() != nil {
					commandErr = errors.New("resume a goal in Agent mode when the session is idle")
				} else if commandErr = conversation.Goal.Resume(); commandErr == nil {
					last, commandErr = conversation.Start(ctx, mode, "Resume the active goal.")
				}
			default:
				if active := conversation.Session.Active(); active != nil {
					commandErr = active.Steer(models.NewUserMessage(line))
				} else {
					last, commandErr = conversation.Start(ctx, mode, line)
				}
			}
			if commandErr != nil {
				fmt.Fprintln(os.Stderr, commandErr)
			}
		}
	}
}

func scan(ctx context.Context, lines chan<- string, result chan<- error) {
	defer close(lines)
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 4096), 1<<20)
	for reader.Scan() {
		select {
		case <-ctx.Done():
			result <- ctx.Err()
			return
		case lines <- reader.Text():
		}
	}
	result <- reader.Err()
}

func display(ctx context.Context, conversation *core.Session, finished chan string) {
	defer close(finished)
	for cursor := uint64(0); ; {
		event, err := conversation.NextEvent(ctx, cursor)
		if err != nil {
			if ctx.Err() == nil {
				fmt.Fprintln(os.Stderr, err)
			}
			return
		}
		cursor = event.Sequence
		switch event.Kind {
		case "text_delta":
			var part models.StreamPart
			if json.Unmarshal(event.Data, &part) == nil {
				fmt.Print(part.Delta)
			}
		case "input.requested", "plan.updated", "goal.updated", "task.updated", "tool.call":
			fmt.Printf("\n[%s] %s\n", event.Kind, event.Data)
		case "run.completed", "run.cancelled", "run.failed":
			fmt.Println()
			if event.Kind == "run.failed" {
				fmt.Fprintf(os.Stderr, "%s\n", event.Data)
			}
			select {
			case <-finished:
			default:
			}
			finished <- event.RunID
		}
	}
}
