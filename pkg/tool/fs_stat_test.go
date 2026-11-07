package tool_test

import (
	toolapi "clara-code/pkg/tool"
	"clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/memory"
	"context"
	"strings"
	"testing"
	"time"
)

type timestampReader struct {
	fs.Reader
	modified time.Time
}

func (r timestampReader) Stat(ctx context.Context, path fs.Path) (fs.Entry, error) {
	entry, err := r.Reader.Stat(ctx, path)
	entry.ModTime = r.modified
	return entry, err
}

func TestStatTimePreservesInstantOffsetAndPrecision(t *testing.T) {
	storage := memory.New()
	if _, err := storage.WriteFile(t.Context(), "/sample", []byte("text"), fs.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, stamp := range []string{"2026-11-08T01:02:03.123456789+08:00", "2026-11-07T12:02:03.12345-05:00", "2026-11-07T17:02:03Z"} {
		modified, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			t.Fatal(err)
		}
		tool := toolapi.NewFiles(timestampReader{Reader: storage, modified: modified}, nil).Stat()
		result := callTool(t, tool, map[string]any{"path": "/sample"})
		if result.IsError {
			t.Fatal(result.Content)
		}
		found := false
		for _, line := range strings.Split(result.Content, "\n") {
			if raw, ok := strings.CutPrefix(line, "modified: "); ok {
				parsed, err := time.Parse(time.RFC3339Nano, raw)
				if err != nil || !parsed.Equal(modified) || raw != stamp {
					t.Fatalf("stat changed timestamp %q: %s, %v", stamp, raw, err)
				}
				found = true
			}
		}
		if !found || !strings.Contains(tool.Info().Description, "RFC3339") {
			t.Fatal("timestamp contract missing", result.Content)
		}
	}
}
