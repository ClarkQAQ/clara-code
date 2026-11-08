package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clara-code/pkg/core"
)

func TestDiagnosticLogUsesRFC3339WithPrecision(t *testing.T) {
	var output bytes.Buffer
	stamp := "2026-11-08T01:02:03.123456789+05:45"
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		t.Fatal(err)
	}
	logger := diagnosticLogger(&output)
	if err := logger.Handler().Handle(t.Context(), slog.NewRecord(at, slog.LevelInfo, "fixture", 0)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "time="+stamp+" ") {
		t.Fatal("log lost its time zone or precision", output.String())
	}
}

func TestACPModelClockAndPTYStatusFollowTZ(t *testing.T) {
	var requests atomic.Int32
	before := time.Now()
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var payload struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		var clock string
		for _, message := range payload.Messages {
			var text string
			if json.Unmarshal(message.Content, &text) != nil {
				var blocks []struct{ Text string }
				if err := json.Unmarshal(message.Content, &blocks); err != nil {
					t.Error(err)
				}
				for _, block := range blocks {
					text += block.Text + "\n"
				}
			}
			if strings.Contains(text, "<environment_context>") {
				clock = text
			}
		}
		zone, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(clock, "<current_date>"+before.In(zone).Format(time.DateOnly)+"</current_date>") && !strings.Contains(clock, "<current_date>"+time.Now().In(zone).Format(time.DateOnly)+"</current_date>") {
			t.Error("ACP model received incorrect local date", clock)
		}
		if !strings.Contains(clock, "Asia/Shanghai (UTC+08:00)") || strings.Contains(clock, "<timestamp>") || strings.Contains(clock, "<local_datetime>") || strings.Contains(clock, "<message_context>") {
			t.Error("ACP model environment contains an unexpected clock", clock)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: "+`{"id":"clock","choices":[{"index":0,"delta":{"content":"Local clock verified."},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	defer endpoint.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client, err := core.ConnectACPExternal(ctx, core.ACPExternalConfig{
		Command: os.Args[0], Args: []string{"acp"},
		Env: map[string]string{childEnv: "1", "CLARA_CODE_HOME": t.TempDir(), "TZ": "Asia/Shanghai",
			"CLARA_CODE_ENDPOINT": endpoint.URL + "/v1", "CLARA_CODE_PROTOCOL": "completions", "CLARA_CODE_MODEL": "clock-fixture", "CLARA_CODE_API_KEY": "fixture"},
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.NewSession(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(ctx, id)
	response, _, err := client.Prompt(ctx, id, "Check the local clock.")
	if err != nil || response != "Local clock verified." || requests.Load() != 1 {
		t.Fatal("ACP clock flow failed", response, err, requests.Load())
	}
	t.Setenv("TZ", "Asia/Shanghai")
	p := startPTY(t, t.TempDir(), t.TempDir())
	p.await(t, 0, "Ready")
	p.send(t, "/status\r")
	p.await(t, 0, "Time zone  Asia/Shanghai (UTC+08:00)")
	p.await(t, 0, "Local time  ")
	p.send(t, "/exit\r")
	p.await(t, 0, "\x1b[?2004l\x1b[?25h")
}
