package coding

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"clara-core/agent/prompt"

	"clara-code/internal/session"
)

func remoteFixture(t *testing.T, handler http.HandlerFunc) (*Conversation, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	c.Host.Config.Endpoint = server.URL + "/v1"
	c.Host.Config.Protocol = "completions"
	key := "fixture-api-key-should-never-appear"
	path := filepath.Join(c.Host.Config.Home, "key")
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	c.Host.Config.APIKeyFile = path
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
	for _, test := range []struct {
		name string
		auth bool
		kind string
	}{{"authentication", true, "authentication"}, {"stream", false, "stream"}} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			c, key := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if test.auth {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, `{"error":{"message":"invalid fixture-api-key-should-never-appear","type":"authentication_error"}}`)
				} else {
					sendTextSSE(w, "Keep this partial reply.", false)
				}
			})
			run, err := c.Start(t.Context(), prompt.Execute, "failure probe", false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := run.Wait(t.Context()); err == nil {
				t.Fatal("expected request failure")
			}
			if requests.Load() != 1 {
				t.Fatalf("unexpected re-execution after failure: %d", requests.Load())
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
			if !test.auth {
				found := false
				for _, cell := range turns[0].Cells {
					if cell.Kind == "assistant" && strings.Contains(cell.Text, "Keep this partial reply.") {
						found = true
					}
				}
				if !found {
					t.Fatal("partial stream was discarded")
				}
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
