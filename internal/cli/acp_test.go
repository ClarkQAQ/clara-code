package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"clara-code/pkg/core"
)

func requireACPCode(t *testing.T, err error, code int) {
	t.Helper()
	var rpcErr *sdk.RequestError
	if !errors.As(err, &rpcErr) || rpcErr.Code != code {
		t.Fatalf("expected ACP error %d, got %v", code, err)
	}
}

func TestACPListSessions(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	home, project := t.TempDir(), t.TempDir()
	for _, dir := range []string{".git", "first", "second"} {
		if err := os.Mkdir(filepath.Join(project, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	first, err := coding.ResolveWorkspace(filepath.Join(project, "first"))
	if err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(first.Root, "second")
	client, err := core.ConnectACPExternal(ctx, core.ACPExternalConfig{
		Command: os.Args[0], Args: []string{"acp", "--demo"},
		Env: map[string]string{childEnv: "1", "CLARA_CODE_HOME": home},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(ctx, "")
	caps := client.Initialize().AgentCapabilities.SessionCapabilities
	if caps.List == nil || caps.Resume == nil {
		t.Fatalf("missing list/resume capabilities: %+v", caps)
	}
	empty, err := client.ListSessions(ctx, sdk.ListSessionsRequest{})
	if err != nil || empty.Sessions == nil || len(empty.Sessions) != 0 || empty.NextCursor != nil {
		t.Fatalf("empty listing %+v, %v", empty, err)
	}

	var want []sdk.SessionId
	for i := range 102 {
		id := fmt.Sprintf("session_list_%03d", i)
		store := session.New(home, session.Meta{Project: first.Root, Cwd: first.Cwd, FirstPrompt: "Saved task"})
		if err := errors.Join(store.Save(ctx, core.State{ID: id, Revision: 1}), store.Close()); err != nil {
			t.Fatal(err)
		}
		want = append(want, sdk.SessionId(id))
	}
	slices.Reverse(want)
	other, err := client.NewSession(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.ListSessions(ctx, sdk.ListSessionsRequest{Cwd: &first.Cwd})
	if err != nil || len(page.Sessions) != 100 || page.NextCursor == nil {
		t.Fatalf("first page size %d, nextCursor %v, %v", len(page.Sessions), page.NextCursor, err)
	}
	tail, err := client.ListSessions(ctx, sdk.ListSessionsRequest{Cwd: &first.Cwd, Cursor: page.NextCursor})
	if err != nil || len(tail.Sessions) != 2 || tail.NextCursor != nil {
		t.Fatalf("second page %+v, %v", tail, err)
	}
	for i, item := range append(page.Sessions, tail.Sessions...) {
		if item.SessionId != want[i] || item.Cwd != first.Cwd || item.Title == nil || *item.Title != "Saved task" || item.UpdatedAt == nil {
			t.Fatalf("unexpected entry %d: %+v", i, item)
		}
		if _, err := time.Parse(time.RFC3339Nano, *item.UpdatedAt); err != nil {
			t.Fatal(err)
		}
	}
	all, err := client.ListSessions(ctx, sdk.ListSessionsRequest{})
	if err != nil || len(all.Sessions) != 100 || all.Sessions[0].SessionId != other {
		t.Fatalf("unfiltered listing size %d, %v", len(all.Sessions), err)
	}
	otherPage, err := client.ListSessions(ctx, sdk.ListSessionsRequest{Cwd: &second})
	if err != nil || len(otherPage.Sessions) != 1 || otherPage.Sessions[0].SessionId != other {
		t.Fatalf("other directory listing %+v, %v", otherPage, err)
	}
	_, err = client.ListSessions(ctx, sdk.ListSessionsRequest{Cwd: &second, Cursor: page.NextCursor})
	requireACPCode(t, err, -32602)
	bad := "not-a-valid-cursor"
	_, err = client.ListSessions(ctx, sdk.ListSessionsRequest{Cursor: &bad})
	requireACPCode(t, err, -32602)
	relative := "first"
	_, err = client.ListSessions(ctx, sdk.ListSessionsRequest{Cwd: &relative})
	requireACPCode(t, err, -32602)
	_, err = client.ResumeSession(ctx, sdk.ResumeSessionRequest{SessionId: want[0], Cwd: second})
	requireACPCode(t, err, -32002)
}

func TestACPResumeAcrossProcesses(t *testing.T) {
	const previous = "Remember the fixture marker XYZ42."
	const next = "Continue the fixture task."
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		answer := "Marker saved."
		if strings.Contains(string(payload), next) {
			if !strings.Contains(string(payload), previous) || !strings.Contains(string(payload), "Marker saved.") {
				t.Error("resumed prompt did not receive the prior exchange")
			}
			if !strings.Contains(string(payload), "mcp__fixture__echo") {
				t.Error("resumed prompt did not receive the requested MCP tool")
			}
			answer = "Continuing with XYZ42."
		}
		w.Header().Set("Content-Type", "text/event-stream")
		encoded, _ := json.Marshal(answer)
		fmt.Fprintf(w, "data: {\"id\":\"resume\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%s},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", encoded)
	}))
	defer endpoint.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	home, cwd := t.TempDir(), t.TempDir()
	config := core.ACPExternalConfig{Command: os.Args[0], Args: []string{"acp"}, Env: map[string]string{
		childEnv: "1", "CLARA_CODE_HOME": home, "CLARA_CODE_ENDPOINT": endpoint.URL + "/v1",
		"CLARA_CODE_PROTOCOL": "completions", "CLARA_CODE_MODEL": "resume-fixture", "CLARA_CODE_API_KEY": "fixture",
	}}
	first, err := core.ConnectACPExternal(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close(ctx, "")
	id, err := first.NewSession(ctx, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if reply, _, err := first.Prompt(ctx, id, previous); err != nil || reply != "Marker saved." {
		t.Fatalf("initial prompt %q, %v", reply, err)
	}
	if err := first.Close(ctx, id); err != nil {
		t.Fatal(err)
	}

	client, err := core.ConnectACPExternal(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(ctx, id)
	_, err = client.ResumeSession(ctx, sdk.ResumeSessionRequest{SessionId: "session_missing", Cwd: cwd})
	requireACPCode(t, err, -32002)
	_, err = client.ResumeSession(ctx, sdk.ResumeSessionRequest{SessionId: id, Cwd: t.TempDir()})
	requireACPCode(t, err, -32002)
	request := sdk.ResumeSessionRequest{SessionId: id, Cwd: cwd, McpServers: []sdk.McpServer{{
		Stdio: &sdk.McpServerStdio{Name: "fixture", Command: os.Args[0], Args: []string{"__mcp_fixture"},
			Env: []sdk.EnvVariable{{Name: childEnv, Value: "1"}}},
	}}}
	var callers sync.WaitGroup
	for range 2 {
		callers.Go(func() {
			if _, err := client.ResumeSession(ctx, request); err != nil {
				t.Error("concurrent resume:", err)
			}
		})
	}
	callers.Wait()
	_, err = client.ResumeSession(ctx, sdk.ResumeSessionRequest{SessionId: id, Cwd: cwd})
	requireACPCode(t, err, -32602)
	_, err = client.ResumeSession(ctx, sdk.ResumeSessionRequest{SessionId: id, Cwd: t.TempDir()})
	requireACPCode(t, err, -32002)
	updates := client.Updates(id)
	select {
	case update := <-updates:
		t.Fatalf("resume sent an unexpected update: %+v", update)
	default:
	}
	if reply, stop, err := client.Prompt(ctx, id, next); err != nil || reply != "Continuing with XYZ42." || stop != sdk.StopReasonEndTurn {
		t.Fatalf("resumed prompt %q, %q, %v", reply, stop, err)
	}
}
