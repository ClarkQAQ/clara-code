package core

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"

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
