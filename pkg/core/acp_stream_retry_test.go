package core

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"testing/synctest"

	sdk "github.com/coder/acp-go-sdk"
)

func TestACPStreamRetryBeforeAndAfterPreview(t *testing.T) {
	textStart := StreamPart{Type: StreamPartTypeTextStart, ID: "text"}
	thoughtStart := StreamPart{Type: StreamPartTypeReasoningStart, ID: "thought"}
	for _, test := range []struct {
		name        string
		first       []StreamPart
		hostReset   bool
		wantOpens   int
		wantMessage string
		wantThought string
	}{
		{"before preview", []StreamPart{textStart}, false, 2, "replacement", ""},
		{"append-only host before first delta", []StreamPart{textStart}, false, 2, "replacement", ""},
		{"before preview with reset", []StreamPart{textStart}, true, 2, "replacement", ""},
		{"after text", []StreamPart{textStart, {Type: StreamPartTypeTextDelta, ID: "text", Delta: "draft"}}, true, 1, "draft", ""},
		{"after reasoning", []StreamPart{thoughtStart, {Type: StreamPartTypeReasoningDelta, ID: "thought", Delta: "thinking"}}, true, 1, "", "thinking"},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var opens, resets int
				language := streamAttemptModel{open: func() (StreamResponse, error) {
					opens++
					if opens == 1 {
						return streamParts(append(test.first, StreamPart{Type: StreamPartTypeError, Error: io.ErrUnexpectedEOF})...), nil
					}
					return streamParts(textStart,
						StreamPart{Type: StreamPartTypeTextDelta, ID: "text", Delta: "replacement"},
						StreamPart{Type: StreamPartTypeTextEnd, ID: "text"},
						StreamPart{Type: StreamPartTypeFinish, FinishReason: FinishReasonStop}), nil
				}}
				session, err := NewSession(t.Context(), NewAgent(language, WithMaxRetries(1)), nil)
				if err != nil {
					t.Fatal(err)
				}
				binding := ACPBinding{Session: session}
				if test.name == "append-only host before first delta" {
					binding.Configure = func(context.Context) AgentStreamCall {
						return AgentStreamCall{OnTextDelta: func(_, text string) error {
							if text != "replacement" {
								t.Errorf("unexpected host delta %q", text)
							}
							return nil
						}}
					}
				}
				if test.hostReset {
					binding.Configure = func(context.Context) AgentStreamCall {
						return AgentStreamCall{
							OnChunk: func(StreamPart) error { return nil },
							OnStreamReset: func() error {
								resets++
								return nil
							},
						}
					}
				}
				client, id := acpStreamTestClient(t, binding)
				_, stop, err := client.Prompt(t.Context(), id, "hello")
				if test.wantOpens == 2 {
					if err != nil || stop != sdk.StopReasonEndTurn {
						t.Fatalf("retried prompt returned stop=%q err=%v", stop, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), io.ErrUnexpectedEOF.Error()) {
					t.Fatalf("interrupted prompt returned %v", err)
				}
				synctest.Wait()
				wantResets := 0
				if test.hostReset {
					wantResets = test.wantOpens - 1
				}
				if opens != test.wantOpens || resets != wantResets {
					t.Fatalf("model opened %d times, host reset %d times", opens, resets)
				}
				updates := client.Updates(id)
				if len(updates) != 1 {
					t.Fatalf("client received %d updates, want one", len(updates))
				}
				update := <-updates
				var message, thought string
				if chunk := update.AgentMessageChunk; chunk != nil && chunk.Content.Text != nil {
					message = chunk.Content.Text.Text
				}
				if chunk := update.AgentThoughtChunk; chunk != nil && chunk.Content.Text != nil {
					thought = chunk.Content.Text.Text
				}
				if message != test.wantMessage || thought != test.wantThought {
					t.Fatalf("client received message %q, thought %q", message, thought)
				}
			})
		})
	}
}

func acpStreamTestClient(t *testing.T, binding ACPBinding) (*ACPExternal, sdk.SessionId) {
	t.Helper()
	server, err := NewACPServer(ACPServerConfig{
		Open: func(context.Context, string, string, []sdk.McpServer) (ACPBinding, error) {
			return binding, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, peer := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), agent, agent) }()
	t.Cleanup(func() {
		_ = peer.Close()
		_ = agent.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	callbacks := &clientCallbacks{updates: make(map[sdk.SessionId]*sessionBuffer)}
	conn := sdk.NewClientSideConnection(callbacks, peer, peer)
	info, err := conn.Initialize(t.Context(), sdk.InitializeRequest{ProtocolVersion: sdk.ProtocolVersionNumber})
	if err != nil {
		t.Fatal(err)
	}
	client := &ACPExternal{conn: conn, client: callbacks, info: info}
	id, err := client.NewSession(t.Context(), "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	return client, id
}
