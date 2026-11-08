package setting

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamIdleTimeoutAndRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	for _, client := range []requestClient{{idle: 30 * time.Millisecond}, {timeout: 30 * time.Millisecond}} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err == nil || !strings.Contains(string(data), "first") || time.Since(start) > time.Second {
			t.Fatal("request did not terminate after timeout", err)
		}
	}
}
