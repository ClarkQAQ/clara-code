package core

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	sdk "github.com/coder/acp-go-sdk"

	"clara-code/pkg/model"
	"clara-code/pkg/util/errs"
)

type acpRequestCounter struct {
	io.Writer
	writes atomic.Int64
}

func (w *acpRequestCounter) Write(p []byte) (int, error) {
	w.writes.Add(1)
	return w.Writer.Write(p)
}

func TestACPClientSkipsUnadvertisedMethods(t *testing.T) {
	server, err := NewACPServer(ACPServerConfig{
		Open: func(context.Context, string, string, []sdk.McpServer) (ACPBinding, error) {
			return ACPBinding{}, errors.New("unexpected session open")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	agent, peer := net.Pipe()
	defer agent.Close()
	defer peer.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, agent, agent) }()
	callbacks := &clientCallbacks{}
	writer := &acpRequestCounter{Writer: peer}
	conn := sdk.NewClientSideConnection(callbacks, writer, peer)
	info, err := conn.Initialize(ctx, sdk.InitializeRequest{ProtocolVersion: sdk.ProtocolVersionNumber})
	if err != nil {
		t.Fatal(err)
	}
	client := &ACPExternal{conn: conn, client: callbacks, info: info}
	before := writer.writes.Load()
	list := sdk.ListSessionsRequest{}
	resume := sdk.ResumeSessionRequest{SessionId: "session_fixture", Cwd: t.TempDir()}
	if _, err := client.ListSessions(ctx, list); !errs.IsCode(err, errs.CodeUnsupported) {
		t.Fatalf("list without support: %v", err)
	}
	if _, err := client.ResumeSession(ctx, resume); !errs.IsCode(err, errs.CodeUnsupported) {
		t.Fatalf("resume without support: %v", err)
	}
	if writer.writes.Load() != before {
		t.Fatal("client sent an unadvertised request")
	}
	// A peer that bypasses negotiation still gets a protocol error.
	_, listErr := conn.ListSessions(ctx, list)
	_, resumeErr := conn.ResumeSession(ctx, resume)
	for _, err := range []error{listErr, resumeErr} {
		var rpcErr *sdk.RequestError
		if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
			t.Fatalf("expected method not found, got %v", err)
		}
	}
	if err := peer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestACPLoadOmitsInternalTurnInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		const id = "session_fixture"
		continuation := model.NewUserMessage("Continue the active goal.")
		continuation.Metadata = &model.MessageMetadata{Kind: model.MessageKindInternalTurn}
		store := NewMemoryStore()
		state := State{ID: id, Revision: 1, Messages: []model.Message{
			model.NewUserMessage("Verify the goal"),
			continuation,
			{Role: model.MessageRoleAssistant, Content: []model.MessagePart{model.TextPart{Text: "Verified."}}},
		}}
		if err := store.Save(ctx, state); err != nil {
			t.Fatal(err)
		}
		server, err := NewACPServer(ACPServerConfig{LoadSession: true,
			Open: func(ctx context.Context, id, _ string, _ []sdk.McpServer) (ACPBinding, error) {
				restored, err := OpenSession(ctx, NewAgent(nil), store, id)
				return ACPBinding{Session: restored}, err
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		agent, peer := net.Pipe()
		done := make(chan error, 1)
		go func() { done <- server.Serve(context.Background(), agent, agent) }()
		defer func() {
			_ = peer.Close()
			_ = agent.Close()
			if err := <-done; err != nil {
				t.Error(err)
			}
		}()
		callbacks := &clientCallbacks{updates: make(map[sdk.SessionId]*sessionBuffer)}
		conn := sdk.NewClientSideConnection(callbacks, peer, peer)
		if _, err := conn.Initialize(ctx, sdk.InitializeRequest{ProtocolVersion: sdk.ProtocolVersionNumber}); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.LoadSession(ctx, sdk.LoadSessionRequest{SessionId: id, Cwd: "/workspace", McpServers: []sdk.McpServer{}}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		updates := callbacks.ensure(id).updates
		if len(updates) != 2 {
			t.Fatalf("got %d history updates, want 2", len(updates))
		}
		user := (<-updates).UserMessageChunk
		reply := (<-updates).AgentMessageChunk
		if user == nil || user.Content.Text == nil || user.Content.Text.Text != "Verify the goal" ||
			reply == nil || reply.Content.Text == nil || reply.Content.Text.Text != "Verified." {
			t.Fatalf("unexpected replay: %+v, %+v", user, reply)
		}
	})
}
