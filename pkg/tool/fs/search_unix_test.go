//go:build darwin || linux

package fs_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	clarafs "clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/local"
	"golang.org/x/sys/unix"
)

func TestSearchSkipsSpecialFilesWithoutOpeningThem(t *testing.T) {
	root, err := os.MkdirTemp("", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", filepath.Join(root, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.go"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	storage, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := clarafs.New(storage).GrepDetailed(ctx, "/", clarafs.SearchOptions{Query: "needle"})
	if err != nil || len(result.Hits) != 1 || result.Skipped != 3 || result.Truncated {
		t.Fatalf("special files: %+v %v", result, err)
	}
}
