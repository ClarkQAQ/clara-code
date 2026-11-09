package tool_test

import (
	"encoding/json"
	"strings"
	"testing"

	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
	"clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/memory"
)

func callTool(t *testing.T, tool model.AgentTool, input any) model.ToolResponse {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Run(t.Context(), model.ToolCall{Input: string(body)})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSearchAndHashlineExamplesExecute(t *testing.T) {
	storage := memory.New()
	for _, name := range []fs.Path{"/internal/main.go", "/main.go"} {
		if _, err := storage.WriteFile(t.Context(), name, []byte("package old\n// cursorPosition\n"), fs.WriteOptions{CreateDirs: true}); err != nil {
			t.Fatal(err)
		}
	}
	files := toolapi.NewFiles(storage, nil)
	glob := callTool(t, files.Glob(), map[string]any{"path": "/", "pattern": "**/*.go"})
	grep := callTool(t, files.Grep(), map[string]any{"path": "/internal", "query": "cursorPosition", "include": []string{"*.go"}})
	if glob.IsError || grep.IsError || !strings.Contains(glob.Content, "/main.go") || !strings.Contains(grep.Content, "/internal/main.go:2:") {
		t.Fatalf("examples: %s %s", glob.Content, grep.Content)
	}
	read := callTool(t, files.Read(), map[string]any{"path": "/main.go", "anchored": true})
	header, _, _ := strings.Cut(read.Content, "\n")
	edit := callTool(t, files.Edit(), map[string]any{"input": header + "\nPUT 1.=1:\n+package main"})
	if read.IsError || edit.IsError {
		t.Fatalf("anchored example: %s %s", read.Content, edit.Content)
	}
	body, err := storage.ReadFile(t.Context(), "/main.go", 100)
	if err != nil || !strings.HasPrefix(string(body), "package main\n") {
		t.Fatalf("edit not applied: %q %v", body, err)
	}
	bad := callTool(t, files.Grep(), map[string]any{"pattern": "cursorPosition"})
	if !bad.IsError || !strings.Contains(bad.Content, "grep: invalid arguments") || !strings.Contains(bad.Content, "query") || !strings.Contains(bad.Content, "Allowed fields:") {
		t.Fatal(bad.Content)
	}
	for _, input := range []string{"null", "[]", "42", "{"} {
		result, err := files.Grep().Run(t.Context(), model.ToolCall{Input: input})
		if err != nil || !result.IsError || !strings.Contains(result.Content, "grep: invalid arguments") {
			t.Fatalf("invalid %s: %+v %v", input, result, err)
		}
	}
}

func TestHashlineDeletionAndRecovery(t *testing.T) {
	const path = "/main.go"
	storage := memory.New()
	if _, err := storage.WriteFile(t.Context(), path, []byte("package main\n// remove me\n// keep me\n"), fs.WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	files := toolapi.NewFiles(storage, nil)
	readLine := func(line int) string {
		t.Helper()
		read := callTool(t, files.Read(), map[string]any{"path": path, "anchored": true, "offset": line, "limit": 1})
		header, _, _ := strings.Cut(read.Content, "\n")
		if read.IsError || !strings.HasPrefix(header, "["+path+"#") {
			t.Fatalf("expected snapshot header, got %q", read.Content)
		}
		return header
	}
	header := readLine(1)
	files = toolapi.NewFiles(storage, nil) // The old transcript can outlive its snapshot store.
	edit := func(operation string) model.ToolResponse {
		return callTool(t, files.Edit(), map[string]any{"input": header + "\n" + operation})
	}
	assertError := func(operation, hint string) {
		t.Helper()
		result := edit(operation)
		if !result.IsError || !strings.Contains(result.Content, hint) {
			t.Fatalf("%q: expected error with %q, got %+v", operation, hint, result)
		}
	}

	assertError("CUT 2.=2", "call read with anchored=true")
	header = readLine(1)
	assertError("CUT 2", "Use CUT 12.=12")
	assertError("CUT 2.=2:", "no colon or body")
	assertError("PUT 2.=2:", "use a lone + for a blank line")
	assertError("CUT 2.=2", "offset/limit covering the target lines")

	header = readLine(2)
	if result := edit("CUT 2.=2"); result.IsError {
		t.Fatal(result.Content)
	}
	assertError("CUT 2.=2", "call read with anchored=true")
	body, err := storage.ReadFile(t.Context(), path, 100)
	if err != nil || string(body) != "package main\n// keep me\n" {
		t.Fatalf("unexpected file after deletion: %q, %v", body, err)
	}
}

func TestCommandUnitsAndStrictArguments(t *testing.T) {
	commands := toolapi.NewCommands(toolapi.NewLocal())
	defer commands.Close()
	wrong := callTool(t, commands.ExecCommand(), map[string]any{"command": "exit 0", "yield_seconds": 1})
	if !wrong.IsError || !strings.Contains(wrong.Content, "exec_command: invalid arguments") || !strings.Contains(wrong.Content, "yield_time_ms") {
		t.Fatal(wrong.Content)
	}
	run := callTool(t, commands.ExecCommand(), map[string]any{"command": "sleep 1.2; printf example-ok", "yield_time_ms": 1000, "timeout_seconds": 120})
	if run.IsError || !strings.HasPrefix(run.Content, "session_id: ") {
		t.Fatal(run.Content)
	}
	first, _, _ := strings.Cut(run.Content, "\n")
	id := strings.TrimPrefix(first, "session_id: ")
	poll := callTool(t, commands.WriteStdin(), map[string]any{"session_id": id, "yield_time_ms": 1000})
	if poll.IsError || !strings.Contains(poll.Content, "example-ok") {
		t.Fatal(poll.Content)
	}
	completed := callTool(t, commands.WriteStdin(), map[string]any{"session_id": id, "yield_time_ms": 1000})
	if !completed.IsError || !strings.Contains(completed.Content, "completed") {
		t.Fatal(completed.Content)
	}
}

func TestIncompleteSearchDoesNotClaimAbsence(t *testing.T) {
	storage := memory.New()
	if _, err := storage.WriteFile(t.Context(), "/large", []byte(strings.Repeat("a", (4<<20)+1)), fs.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	result := callTool(t, toolapi.NewFiles(storage, nil).Grep(), map[string]any{"query": "needle"})
	if result.IsError || !strings.Contains(result.Content, "Search incomplete") || strings.Contains(result.Content, "No matches.\n") {
		t.Fatal(result.Content)
	}
}
