package coding

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"clara-code/internal/session"
	"clara-code/internal/setting"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func TestLocalCompactionThroughProvider(t *testing.T) {
	for _, protocol := range []string{"responses", "completions"} {
		for _, outcome := range []string{"complete", "trim", "length"} {
			t.Run(protocol+"/"+outcome, func(t *testing.T) {
				limit := int64(0)
				if outcome != "complete" {
					limit = 8192
				}
				var attempts atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempt := attempts.Add(1)
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					field, maxField := "input", "max_output_tokens"
					if protocol == "completions" {
						field, maxField = "messages", "max_tokens"
					}
					if limit == 0 && body[maxField] != nil || limit != 0 && string(body[maxField]) != fmt.Sprint(limit) {
						t.Errorf("%s = %s for configured limit %d", maxField, body[maxField], limit)
					}
					if len(body["max_completion_tokens"]) > 0 || len(body["tools"]) > 0 && string(body["tools"]) != "[]" {
						t.Errorf("unexpected tools or token override: %s %s", body["tools"], body["max_completion_tokens"])
					}
					var items []struct {
						Role    string
						Content json.RawMessage
					}
					if err := json.Unmarshal(body[field], &items); err != nil || len(items) == 0 {
						t.Errorf("invalid message list: %s", body[field])
					} else if last := items[len(items)-1]; last.Role != "user" || !strings.Contains(string(last.Content), "Write a handoff for this test.") {
						t.Errorf("unexpected final instruction: %+v", last)
					}
					text := string(body[field])
					if !strings.Contains(text, "latest requirement") || strings.Contains(text, "oldest observation") != (attempt == 1) {
						t.Errorf("incorrect history on attempt %d: %s", attempt, text)
					}
					w.Header().Set("Content-Type", "application/json")
					if attempt == 1 && outcome == "trim" {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","message":"Input exceeds the window."}}`)
						return
					}
					if protocol == "completions" {
						reason := "stop"
						if outcome == "length" {
							reason = "length"
						}
						fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":"summary text"},"finish_reason":%q}]}`, reason)
					} else if outcome == "length" {
						fmt.Fprint(w, `{"status":"incomplete","incomplete_details":{"reason":"length"},"output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"thinking"}]}]}`)
					} else {
						fmt.Fprint(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"summary text"}]}]}`)
					}
				}))
				defer server.Close()
				h := Host{Config: setting.Config{Endpoint: server.URL, Protocol: protocol, APIKey: "test", DefaultModel: "fixture",
					Models: map[string]setting.Model{"fixture": {ID: "fixture", MaxOutputTokens: limit}}}}
				m, err := h.model(t.Context(), "fixture", "")
				if err != nil {
					t.Fatal(err)
				}
				history := []model.Message{model.NewUserMessage("oldest observation"),
					{Role: model.MessageRoleAssistant, Content: []model.MessagePart{model.TextPart{Text: "checked the logs"}}},
					model.NewUserMessage("latest requirement")}
				before, _ := json.Marshal(history)
				compactor := prompt.Compactor{Force: true, Instructions: "Write a handoff for this test."}
				result, err := compactor.Fit(t.Context(), m, "", history, nil)
				after, _ := json.Marshal(history)
				if string(before) != string(after) {
					t.Fatal("summary attempts mutated the original history")
				}
				if outcome == "length" {
					if err == nil || result != nil || attempts.Load() != 1 {
						t.Fatalf("truncated summary: result=%v attempts=%d err=%v", result, attempts.Load(), err)
					}
					return
				}
				expected := int32(1)
				if outcome == "trim" {
					expected++
				}
				if err != nil || len(result) == 0 || attempts.Load() != expected || !strings.Contains(session.Text(result[len(result)-1]), "summary text") {
					t.Fatalf("compaction: history=%v attempts=%d err=%v", result, attempts.Load(), err)
				}
			})
		}
	}
}
