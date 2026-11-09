package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func imagePNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 9, 7))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestACPImageReachesModelEndpoint(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !strings.Contains(fmt.Sprint(body), "data:image/png;base64,") {
			t.Error("ACP image missing on model wire")
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: "+`{"id":"image","choices":[{"index":0,"delta":{"content":"Image verified."},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "acp")
	cmd.Env = append(os.Environ(), childEnv+"=1", "CLARA_CODE_HOME="+t.TempDir(), "CLARA_CODE_ENDPOINT="+server.URL+"/v1", "CLARA_CODE_PROTOCOL=completions", "CLARA_CODE_MODEL=vision", "CLARA_CODE_API_KEY=fixture")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); cancel(); cmd.Wait() }()
	scanner := bufio.NewScanner(output)
	rpc := func(id int, method string, params any) map[string]any {
		t.Helper()
		if err := json.NewEncoder(input).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		for scanner.Scan() {
			var response map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response["id"] == float64(id) {
				if response["error"] != nil {
					t.Fatal(response["error"])
				}
				result, _ := response["result"].(map[string]any)
				return result
			}
		}
		t.Fatal("ACP response missing", scanner.Err())
		return nil
	}
	rpc(1, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}})
	result := rpc(2, "session/new", map[string]any{"cwd": t.TempDir(), "mcpServers": []any{}})
	rpc(3, "session/prompt", map[string]any{"sessionId": result["sessionId"], "prompt": []any{map[string]any{"type": "image", "mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(imagePNG(t))}}})
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
