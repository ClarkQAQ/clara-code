package host_test

import (
	"os"
	"path/filepath"
	"testing"

	corefs "clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/host"
	"clara-code/pkg/tool/fs/local"
	"clara-code/pkg/util/errs"
)

func TestNativePathsAndSymlinkOperations(t *testing.T) {
	base := t.TempDir()
	project, outside := filepath.Join(base, "project"), filepath.Join(base, "outside")
	for _, dir := range []string{project, outside} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	storage, err := host.New(project)
	if err != nil {
		t.Fatal(err)
	}
	absolute := func(value string) corefs.Path {
		p, err := storage.ResolvePath(value)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	target := absolute("../outside/target.txt")
	if _, err := storage.WriteFile(t.Context(), target, []byte("original"), corefs.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	body, err := storage.ReadFile(t.Context(), target, 100)
	if err != nil || string(body) != "original" {
		t.Fatalf("outside read %q %v", body, err)
	}
	link := filepath.Join(project, "link.txt")
	if err := os.Symlink(filepath.FromSlash(target.String()), link); err != nil {
		t.Fatal(err)
	}
	linked := absolute(link)
	if _, err := storage.WriteFile(t.Context(), linked, []byte("updated"), corefs.WriteOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link replaced %v %v", info, err)
	}
	body, err = storage.ReadFile(t.Context(), target, 100)
	if err != nil || string(body) != "updated" {
		t.Fatalf("target not updated %q %v", body, err)
	}
	if _, err := storage.WriteFile(t.Context(), linked, []byte("oops"), corefs.WriteOptions{}); !errs.IsCode(err, errs.CodeConflict) {
		t.Fatalf("link create: %v", err)
	}
	moved := absolute("../outside/moved-link")
	if err := storage.Rename(t.Context(), linked, moved); err != nil {
		t.Fatal(err)
	}
	if err := storage.Remove(t.Context(), moved, corefs.RemoveOptions{Recursive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.FromSlash(target.String())); err != nil {
		t.Fatal("remove followed link", err)
	}
	if err := storage.Remove(t.Context(), target, corefs.RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.ReadFile(t.Context(), target, 100); !errs.IsCode(err, errs.CodeNotFound) {
		t.Fatalf("missing path %v", err)
	}
	t.Setenv("HOME", outside)
	t.Setenv("USERPROFILE", outside)
	if p := absolute("~/new.txt"); p != corefs.Path(filepath.ToSlash(filepath.Join(outside, "new.txt"))) {
		t.Fatal(p)
	}
	if p := absolute("~"); p != corefs.Path(filepath.ToSlash(outside)) {
		t.Fatal(p)
	}
	if p := absolute("/"); p != "/" {
		t.Fatal(p)
	}
	// The original local backend keeps its workspace boundary.
	confined, err := local.New(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := confined.Stat(t.Context(), "../outside"); !errs.IsCode(err, errs.CodeForbidden) {
		t.Fatal("local boundary changed", err)
	}
}
