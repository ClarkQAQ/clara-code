package coding

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"clara-code/pkg/core/prompt"

	"clara-code/internal/session"
	"clara-code/internal/setting"
)

func remoteFixture(t *testing.T, handler http.HandlerFunc) (*Conversation, string) {
	t.Helper()
	return remoteProtocolFixture(t, "completions", nil, handler)
}

func remoteProtocolFixture(t *testing.T, protocol string, retries *int, handler http.HandlerFunc) (*Conversation, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	c.Host.Config.Endpoint = server.URL + "/v1"
	c.Host.Config.Protocol = protocol
	key := "fixture-api-key-should-never-appear"
	c.Host.Config.APIKey = key
	if retries != nil {
		connection := c.Host.Config.Connection()
		connection.RequestMaxRetries = retries
		c.Host.Config.ModelProvider = "fixture"
		c.Host.Config.Providers = map[string]setting.Provider{"fixture": connection}
	}
	factory, err := c.Host.Config.Factory()
	if err != nil {
		t.Fatal(err)
	}
	c.Host.Models = factory
	if err := c.ChangeModel(t.Context(), "fixture", ""); err != nil {
		t.Fatal(err)
	}
	return c, key
}

func sendTextSSE(w http.ResponseWriter, text string, finish bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: %s\n\n", `{"id":"reply","object":"chat.completion.chunk","created":1,"model":"fixture","choices":[{"index":0,"delta":{"role":"assistant","content":`+fmt.Sprintf("%q", text)+`},"finish_reason":null}]}`)
	if finish {
		fmt.Fprint(w, "data: "+`{"id":"reply","object":"chat.completion.chunk","created":1,"model":"fixture","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`+"\n\ndata: [DONE]\n\n")
	}
}

func TestRetryEventsRetainReasonAndRecover(t *testing.T) {
	var requests atomic.Int32
	c, key := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "0.001")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":{"message":"temporary fixture-api-key-should-never-appear","type":"server_error"}}`)
			return
		}
		sendTextSSE(w, "Recovered.", true)
	})
	finish(t, c, prompt.Execute, "retry probe")
	if requests.Load() != 2 {
		t.Fatalf("requests %d", requests.Load())
	}
	state, err := c.Store.Load(t.Context(), c.Session.ID())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range state.Events {
		if strings.Contains(string(event.Data), key) {
			t.Fatal("credential appeared in persisted diagnostics")
		}
		if event.Kind == "model.retry" {
			var retry session.RetryStatus
			if err := json.Unmarshal(event.Data, &retry); err != nil {
				t.Fatal(err)
			}
			if retry.Attempt != 1 || retry.Limit != 3 || retry.Kind != "server" || retry.Message == "" {
				t.Fatalf("retry %+v", retry)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("retry status missing")
	}
}

func TestAuthenticationFailureAndInterruptedStream(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		kind     string
		retries  int
		requests int32
	}{
		{"authentication", "authentication", 3, 1},
		{"stream retries disabled", "stream", 0, 1},
		{"stream retries exhausted", "stream", 1, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			c, key := remoteProtocolFixture(t, "completions", &test.retries, func(w http.ResponseWriter, r *http.Request) {
				attempt := requests.Add(1)
				if test.kind == "authentication" {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, `{"error":{"message":"invalid fixture-api-key-should-never-appear","type":"authentication_error"}}`)
				} else {
					sendTextSSE(w, fmt.Sprintf("Partial reply from attempt %d.", attempt), false)
				}
			})
			run, err := c.Start(t.Context(), prompt.Execute, "failure probe", false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := run.Wait(t.Context()); err == nil {
				t.Fatal("expected request failure")
			}
			if requests.Load() != test.requests {
				t.Fatalf("got %d requests, want %d", requests.Load(), test.requests)
			}
			turns, err := c.Store.Turns()
			if err != nil || len(turns) != 1 {
				t.Fatalf("turns %d %v", len(turns), err)
			}
			if turns[0].Failure == nil || turns[0].Failure.Kind != test.kind {
				t.Fatalf("failure %+v", turns[0].Failure)
			}
			state, err := c.Store.Load(t.Context(), c.Session.ID())
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range state.Events {
				if strings.Contains(string(event.Data), key) {
					t.Fatal("credential leaked into failure records")
				}
			}
			if test.kind == "stream" {
				want := fmt.Sprintf("Partial reply from attempt %d.", test.requests)
				if got := assistantReplies(turns[0]); !slices.Equal(got, []string{want}) {
					t.Fatalf("transcript assistant replies %q, want only %q", got, want)
				}
			}
		})
	}
}

func assistantReplies(turn session.Turn) []string {
	var replies []string
	for _, cell := range turn.Cells {
		if cell.Kind == "assistant" {
			replies = append(replies, cell.Text)
		}
	}
	return replies
}

func TestInterruptedStreamRetriesSameStep(t *testing.T) {
	t.Parallel()
	const partial = "Discard this interrupted reply."
	const recovered = "The retried reply is complete."
	for _, protocol := range []struct {
		name string
		send func(http.ResponseWriter, string, bool)
	}{
		{"completions", sendTextSSE},
		{"responses", sendResponseTextSSE},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			var bodies []string
			c, _ := remoteProtocolFixture(t, protocol.name, nil, func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				mu.Lock()
				bodies = append(bodies, string(body))
				attempt := len(bodies)
				mu.Unlock()
				switch attempt {
				case 1:
					protocol.send(w, partial, false)
				case 2:
					protocol.send(w, recovered, true)
				case 3:
					protocol.send(w, "Follow-up complete.", true)
				default:
					t.Errorf("unexpected request %d", attempt)
					w.WriteHeader(http.StatusBadRequest)
				}
			})

			run, err := c.Start(t.Context(), prompt.Execute, "Recover the interrupted stream.", false)
			if err != nil {
				t.Fatal(err)
			}
			result, err := run.Wait(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Steps) != 1 || result.Response.Content.Text() != recovered {
				t.Fatalf("got %d steps and final reply %q", len(result.Steps), result.Response.Content.Text())
			}
			turns, err := c.Store.Turns()
			if err != nil || len(turns) != 1 {
				t.Fatalf("turns %d, error %v", len(turns), err)
			}
			if got := assistantReplies(turns[0]); turns[0].Failure != nil || !slices.Equal(got, []string{recovered}) {
				t.Fatalf("transcript assistant replies %q, failure %+v", got, turns[0].Failure)
			}
			state, err := c.Store.Load(t.Context(), c.Session.ID())
			if err != nil {
				t.Fatal(err)
			}
			var retries []session.RetryStatus
			for _, event := range state.Events {
				switch event.Kind {
				case "model.error":
					t.Fatalf("recovered turn recorded an error: %s", event.Data)
				case "model.retry":
					var retry session.RetryStatus
					if err := json.Unmarshal(event.Data, &retry); err != nil {
						t.Fatal(err)
					}
					retries = append(retries, retry)
				}
			}
			if len(retries) != 1 || retries[0].Attempt != 1 || retries[0].Kind != "stream" || retries[0].Limit != 3 {
				t.Fatalf("unexpected retries %+v", retries)
			}

			finish(t, c, prompt.Execute, "Continue from the completed reply.")
			mu.Lock()
			defer mu.Unlock()
			if len(bodies) != 3 {
				t.Fatalf("got %d requests, want two attempts and one follow-up", len(bodies))
			}
			if bodies[0] != bodies[1] {
				t.Error("retry changed the original request")
			}
			if strings.Contains(bodies[2], partial) || strings.Count(bodies[2], recovered) != 1 {
				t.Error("follow-up history must contain only the successful assistant reply")
			}
		})
	}
}

func TestRateLimitExhaustsTheCoreRetryBudget(t *testing.T) {
	var requests atomic.Int32
	c, _ := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "0.001")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"fixture rate limit","type":"rate_limit_error"}}`)
	})
	run, err := c.Start(t.Context(), prompt.Execute, "retry until the budget is exhausted", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(t.Context()); err == nil {
		t.Fatal("expected final failure")
	}
	turns, err := c.Store.Turns()
	if err != nil || len(turns) != 1 || turns[0].Failure == nil || turns[0].Failure.Kind != "rate_limit" {
		t.Fatalf("missing final rate limit failure: %+v %v", turns, err)
	}
	if requests.Load() != 4 {
		t.Fatalf("retry budget made %d requests, expected one initial request and three retries", requests.Load())
	}
}
