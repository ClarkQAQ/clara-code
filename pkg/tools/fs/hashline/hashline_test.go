package hashline

import (
	"context"
	"strings"
	"testing"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/tools/fs/memory"
)

func TestEditorReplacesVisibleRangeAndUpdatesTag(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	_, err := storage.WriteFile(ctx, "/main.go", []byte("package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"), writeOptions())
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	snapshot := store.Record("/main.go", "package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n", []int{4})
	editor := &Editor{FS: storage, Store: store, Resolver: DefaultResolver{}}
	result, err := editor.Apply(ctx, "["+"/main.go#"+snapshot.Tag+"]\nPUT 4.=4:\n+\tprintln(\"new\")\n")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := storage.ReadFile(ctx, "/main.go", 1<<20)
	if !strings.Contains(string(data), `println("new")`) {
		t.Fatalf("content = %s", data)
	}
	if result.Files[0].NewTag == snapshot.Tag {
		t.Fatal("tag did not change")
	}
	current, ok := store.Current("/main.go")
	if !ok || current.Tag != result.Files[0].NewTag {
		t.Fatalf("current snapshot = %+v", current)
	}
}

func TestEditorRejectsUnseenAndStaleLines(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	_, _ = storage.WriteFile(ctx, "/a.txt", []byte("one\ntwo\n"), writeOptions())
	store := NewStore()
	snapshot := store.Record("/a.txt", "one\ntwo\n", []int{1})
	editor := &Editor{FS: storage, Store: store}
	_, err := editor.Apply(ctx, "[/a.txt#"+snapshot.Tag+"]\nPUT 2.=2:\n+changed\n")
	if err == nil || !strings.Contains(err.Error(), "not visible") {
		t.Fatalf("err = %v", err)
	}
	_, _ = storage.WriteFile(ctx, "/a.txt", []byte("one\nchanged elsewhere\n"), writeOptions())
	_, err = editor.Apply(ctx, "[/a.txt#"+snapshot.Tag+"]\nPUT 1.=1:\n+changed\n")
	if err == nil || !strings.Contains(err.Error(), "changed after") {
		t.Fatalf("err = %v", err)
	}
}

func TestEditorCutsAndPastesNamedRegister(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	_, _ = storage.WriteFile(ctx, "/a.txt", []byte("a\nb\nc\n"), writeOptions())
	store := NewStore()
	snapshot := store.Record("/a.txt", "a\nb\nc\n", []int{1, 2, 3})
	editor := &Editor{FS: storage, Store: store}
	_, err := editor.Apply(ctx, "[/a.txt#"+snapshot.Tag+"]\nCUT 2.=2 @middle\nPUT >$ @middle\n")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := storage.ReadFile(ctx, "/a.txt", 1<<20)
	if got := string(data); got != "a\nc\nb\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestEditorResolvesGoAndMarkdownBlocks(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	source := "package x\n\nfunc hello() {\n\tprintln(1)\n}\n"
	_, _ = storage.WriteFile(ctx, "/x.go", []byte(source), writeOptions())
	store := NewStore()
	snapshot := store.Record("/x.go", source, []int{3, 4, 5})
	editor := &Editor{FS: storage, Store: store, Resolver: DefaultResolver{}}
	_, err := editor.Apply(ctx, "[/x.go#"+snapshot.Tag+"]\nPUT 3*:\n+func hello() {\n+\tprintln(2)\n+}\n")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := storage.ReadFile(ctx, "/x.go", 1<<20)
	if !strings.Contains(string(data), "println(2)") {
		t.Fatalf("content = %s", data)
	}
}

func TestParseRejectsUnifiedDiff(t *testing.T) {
	_, err := Parse("[a.txt#ABCD]\n@@ -1 +1 @@\n")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEditorMoveWithoutContentChangeRenames(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	_, _ = storage.WriteFile(ctx, "/old.txt", []byte("same\n"), writeOptions())
	store := NewStore()
	snapshot := store.Record("/old.txt", "same\n", []int{1})
	editor := &Editor{FS: storage, Store: store}
	_, err := editor.Apply(ctx, "[/old.txt#"+snapshot.Tag+"]\nMV /new.txt\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Stat(ctx, "/new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Stat(ctx, "/old.txt"); err == nil {
		t.Fatal("old path still exists")
	}
}

func FuzzParse(f *testing.F) {
	f.Add("[a.txt#ABCD]\nPUT 1.=1:\n+hello\n")
	f.Add("*** Begin Patch\n[a.txt#ABCD]\nCUT 1.=1\n*** End Patch")
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = Parse(input)
	})
}

func writeOptions() clarafs.WriteOptions {
	return clarafs.WriteOptions{CreateDirs: true, Overwrite: true}
}
