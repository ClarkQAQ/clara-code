package fs_test

import (
	"context"
	"testing"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/tools/fs/local"
	"clara-code/pkg/tools/fs/memory"
	"clara-code/pkg/tools/fs/mount"
	"clara-code/pkg/tools/fs/restricted"
	"clara-code/pkg/tools/fs/virtual"
	"clara-code/pkg/util/errs"
)

func TestProvidersShareBehavior(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		storage, err := local.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		testProvider(t, storage, "/", true)
	})
	t.Run("memory", func(t *testing.T) {
		testProvider(t, memory.New(), "/", true)
	})
	t.Run("mount", func(t *testing.T) {
		base := memory.New()
		composite := mount.New()
		if err := composite.Mount("/workspace", base); err != nil {
			t.Fatal(err)
		}
		testProvider(t, composite, "/workspace", true)
	})
	t.Run("virtual", func(t *testing.T) {
		base := memory.New()
		ctx := context.Background()
		file := clarafs.Path("/docs/hello.txt")
		if _, err := base.WriteFile(ctx, file, []byte("hello\nworld\n"), clarafs.WriteOptions{
			Mode: 0o644, CreateDirs: true, Overwrite: true,
		}); err != nil {
			t.Fatal(err)
		}
		testProvider(t, virtual.New(base), "/", false)
	})
}

func testProvider(t *testing.T, storage clarafs.FS, root clarafs.Path, writable bool) {
	t.Helper()
	ctx := context.Background()
	file, err := clarafs.Join(root, "docs", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if writable {
		if _, err := storage.WriteFile(ctx, file, []byte("hello\nworld\n"), clarafs.WriteOptions{
			Mode: 0o644, CreateDirs: true, Overwrite: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	entry, err := storage.Stat(ctx, file)
	if err != nil || entry.IsDir || entry.Size != 12 {
		t.Fatalf("stat = %+v, %v", entry, err)
	}
	data, err := storage.ReadFile(ctx, file, 1024)
	if err != nil || string(data) != "hello\nworld\n" {
		t.Fatalf("read = %q, %v", data, err)
	}
	offset, err := storage.ReadAt(ctx, file, 6, 5)
	if err != nil || string(offset) != "world" {
		t.Fatalf("readAt = %q, %v", offset, err)
	}
	dir := file.Dir()
	entries, err := storage.ReadDir(ctx, dir)
	if err != nil || len(entries) != 1 || entries[0].Name != "hello.txt" {
		t.Fatalf("readDir = %+v, %v", entries, err)
	}
	service := clarafs.New(storage)
	hits, err := service.Grep(ctx, root, clarafs.SearchOptions{Query: "world"})
	if err != nil || len(hits) != 1 || hits[0].Line != 2 {
		t.Fatalf("grep = %+v, %v", hits, err)
	}
	if !writable {
		return
	}
	renamed, _ := clarafs.Join(root, "docs", "renamed.txt")
	if err := storage.Rename(ctx, file, renamed); err != nil {
		t.Fatal(err)
	}
	copied, _ := clarafs.Join(root, "copy.txt")
	if err := service.Copy(ctx, renamed, copied); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.ReadFile(ctx, copied, 1024); err != nil {
		t.Fatal(err)
	}
}

func TestRestrictedPolicyBlocksWrites(t *testing.T) {
	base := memory.New()
	storage := restricted.New(base, restricted.Policy{Read: []string{"/public"}, Write: []string{"/public"}})
	ctx := context.Background()
	if _, err := storage.WriteFile(ctx, "/public/a.txt", []byte("a"), clarafs.WriteOptions{CreateDirs: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.WriteFile(ctx, "/private/a.txt", []byte("a"), clarafs.WriteOptions{CreateDirs: true}); !errs.IsCode(err, errs.CodeForbidden) {
		t.Fatalf("err = %v", err)
	}
}

func TestVirtualFilesystemRejectsWrites(t *testing.T) {
	base := memory.New()
	storage := virtual.New(base)
	_, err := storage.WriteFile(context.Background(), "/a.txt", []byte("x"), clarafs.WriteOptions{Overwrite: true})
	if !errs.IsCode(err, errs.CodeForbidden) {
		t.Fatalf("err = %v", err)
	}
}
