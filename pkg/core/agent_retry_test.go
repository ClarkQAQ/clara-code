package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"clara-code/pkg/model"
)

type streamAttemptModel struct {
	LanguageModel
	open func() (StreamResponse, error)
}

func (m streamAttemptModel) Stream(context.Context, Call) (StreamResponse, error) {
	return m.open()
}

func streamParts(parts ...StreamPart) StreamResponse {
	return slices.Values(parts)
}

func TestStreamRetriesFirstErrorChunkWithoutReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cause := fmt.Errorf("upstream closed: %w", io.EOF)
		providerErr := &ProviderError{Cause: cause, Message: "connection closed"}
		var opens, errorsObserved int
		language := streamAttemptModel{open: func() (StreamResponse, error) {
			opens++
			if opens == 1 {
				return streamParts(StreamPart{Type: StreamPartTypeError, Error: providerErr}), nil
			}
			return streamParts(StreamPart{Type: StreamPartTypeFinish, FinishReason: FinishReasonStop}), nil
		}}
		_, err := NewAgent(language, WithMaxRetries(1)).Stream(t.Context(), AgentStreamCall{
			Prompt: "hello",
			OnChunk: func(part StreamPart) error {
				if part.Type == StreamPartTypeError {
					errorsObserved++
					if !errors.Is(part.Error, providerErr) {
						t.Fatal("observer lost the original error part")
					}
				}
				return nil
			},
			OnRetry: func(err *ProviderError, _ time.Duration) {
				if err == nil || !errors.Is(err, cause) || !err.IsRetryable() {
					t.Fatalf("unexpected retry error %v", err)
				}
			},
		})
		if err != nil || opens != 2 || errorsObserved != 1 {
			t.Fatalf("got error=%v opens=%d error chunks=%d", err, opens, errorsObserved)
		}
	})
}

type redactedStreamError struct{ error }

func (e redactedStreamError) Error() string { return "redacted" }
func (e redactedStreamError) Unwrap() error { return e.error }

func TestNormalizeStreamErrorRespectsProviderDiagnostics(t *testing.T) {
	cause := &ProviderError{
		Message: "sensitive diagnostic", ResponseHeaders: map[string]string{"secret": "token"}, Cause: io.EOF,
	}
	var classified *ProviderError
	normalized := normalizeStreamError(redactedStreamError{cause})
	if !errors.As(normalized, &classified) || !errors.Is(normalized, io.EOF) || !classified.IsRetryable() ||
		strings.Contains(classified.Error(), cause.Message) || len(classified.ResponseHeaders) != 0 {
		t.Fatal("normalization must preserve classification and outer redaction")
	}
	for _, provider := range []*ProviderError{
		{StatusCode: 400, Cause: io.EOF}, {AuthError: true, Cause: io.EOF}, {ContextTooLargeErr: true, Cause: io.EOF},
	} {
		normalized := normalizeStreamError(provider)
		if !errors.As(normalized, &classified) || !errors.Is(normalized, provider) || classified.IsRetryable() {
			t.Fatalf("lost the provider error classification for %v", provider)
		}
	}
}

func TestStreamRetriesOpeningAndReadingWithOneBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var trace []string
		var opens int
		language := streamAttemptModel{open: func() (StreamResponse, error) {
			opens++
			trace = append(trace, "open")
			switch opens {
			case 1:
				return nil, fmt.Errorf("opening: %w", io.EOF)
			case 2:
				return streamParts(StreamPart{Type: StreamPartTypeTextDelta, Delta: "draft"}), nil
			case 3:
				return streamParts(StreamPart{Type: StreamPartTypeError, Error: io.ErrUnexpectedEOF}), nil
			default:
				t.Fatal("retry budget exceeded")
				return nil, nil
			}
		}}
		_, err := NewAgent(language, WithMaxRetries(2)).Stream(t.Context(), AgentStreamCall{
			Prompt: "hello",
			OnTextDelta: func(_, text string) error {
				trace = append(trace, text)
				return nil
			},
			OnRetry: func(err *ProviderError, delay time.Duration) {
				if err == nil || !err.IsRetryable() {
					t.Fatalf("retry lost its transport classification: %v", err)
				}
				if strings.Contains(err.Error(), "opening:") {
					t.Fatalf("wrapped EOF escaped the transport diagnostic: %v", err)
				}
				trace = append(trace, delay.String())
			},
			OnStreamReset: func() error {
				trace = append(trace, "reset")
				return nil
			},
		})
		var exhausted *RetryError
		if !errors.As(err, &exhausted) || len(exhausted.Errors) != 3 || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("unexpected failure after %d requests: %v", opens, err)
		}
		if !slices.Equal(trace, []string{"open", "5s", "open", "draft", "10s", "reset", "open"}) {
			t.Fatalf("unexpected attempt sequence %q", trace)
		}
	})
}

func TestStreamResetFailurePreventsReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var opens int
		language := streamAttemptModel{open: func() (StreamResponse, error) {
			opens++
			return streamParts(StreamPart{Type: StreamPartTypeTextStart}), nil
		}}
		resetErr := &ProviderError{StatusCode: 401, TransientError: true}
		_, err := NewAgent(language, WithMaxRetries(3)).Stream(t.Context(), AgentStreamCall{
			Prompt:        "hello",
			OnChunk:       func(StreamPart) error { return nil },
			OnStreamReset: func() error { return resetErr },
			OnAuthRefresh: func(context.Context, *ProviderError) error {
				t.Fatal("unexpected authentication refresh after a reset failure")
				return nil
			},
		})
		if !errors.Is(err, resetErr) || opens != 1 {
			t.Fatalf("reset got err=%v opens=%d", err, opens)
		}
	})
}

func TestStreamRetryPreservesPublishedOutputOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var opens, resets int
	language := streamAttemptModel{open: func() (StreamResponse, error) {
		opens++
		return streamParts(StreamPart{Type: StreamPartTypeTextStart, ID: "partial"}), nil
	}}
	_, err := NewAgent(language, WithMaxRetries(1)).Stream(ctx, AgentStreamCall{
		Prompt:  "hello",
		OnChunk: func(StreamPart) error { return nil },
		OnRetry: func(*ProviderError, time.Duration) { cancel() },
		OnStreamReset: func() error {
			resets++
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) || opens != 1 || resets != 0 {
		t.Fatalf("cancel got err=%v opens=%d resets=%d", err, opens, resets)
	}
}

func TestStreamReplayBoundaries(t *testing.T) {
	errCallback := &ProviderError{StatusCode: 401, TransientError: true, Message: "observer failed"}
	for _, tc := range []struct {
		name   string
		parts  []StreamPart
		call   AgentStreamCall
		wanted error
	}{
		{
			name:   "published without reset",
			parts:  []StreamPart{{Type: StreamPartTypeReasoningStart}},
			call:   AgentStreamCall{OnReasoningStart: func(string, ReasoningContent) error { return nil }},
			wanted: io.ErrUnexpectedEOF,
		},
		{
			name:  "forwarded to append-only client",
			parts: []StreamPart{{Type: StreamPartTypeTextDelta}},
			call: AgentStreamCall{
				OnChunk:       func(StreamPart) error { return nil },
				chunkDelivery: func(StreamPart) chunkDelivery { return chunkCommitted },
			},
			wanted: io.ErrUnexpectedEOF,
		},
		{
			name:   "provider tool started",
			parts:  []StreamPart{{Type: StreamPartTypeToolInputStart, ProviderExecuted: true}},
			wanted: io.ErrUnexpectedEOF,
		},
		{
			name:   "provider tool result",
			parts:  []StreamPart{{Type: model.StreamPartTypeContent, Content: ToolResultContent{ProviderExecuted: true}}},
			wanted: io.ErrUnexpectedEOF,
		},
		{
			name:   "after finish",
			parts:  []StreamPart{{Type: StreamPartTypeFinish}, {Type: StreamPartTypeError, Error: io.EOF}},
			wanted: io.ErrUnexpectedEOF,
		},
		{
			name:   "observer failed",
			parts:  []StreamPart{{Type: StreamPartTypeTextDelta}},
			call:   AgentStreamCall{OnChunk: func(StreamPart) error { return errCallback }},
			wanted: errCallback,
		},
		{
			name:   "checkpoint failed",
			parts:  []StreamPart{{Type: StreamPartTypeFinish}},
			call:   AgentStreamCall{OnModelMessages: func([]Message) error { return errCallback }},
			wanted: errCallback,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var opens int
			language := streamAttemptModel{open: func() (StreamResponse, error) {
				opens++
				return streamParts(tc.parts...), nil
			}}
			call := tc.call
			call.Prompt = "hello"
			call.OnAuthRefresh = func(context.Context, *ProviderError) error {
				t.Fatal("unexpected authentication refresh")
				return nil
			}
			call.OnRetry = func(*ProviderError, time.Duration) { t.Fatal("unexpected retry") }
			if tc.name != "published without reset" {
				call.OnStreamReset = func() error {
					t.Fatal("unexpected reset")
					return nil
				}
			}
			_, err := NewAgent(language, WithMaxRetries(1)).Stream(t.Context(), call)
			if !errors.Is(err, tc.wanted) || opens != 1 {
				t.Fatalf("got error=%v opens=%d", err, opens)
			}
		})
	}
}

func TestStreamRetryDispatchesOnlyCompletedTools(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var opens, executions, checkpoints int
		language := streamAttemptModel{open: func() (StreamResponse, error) {
			opens++
			call := StreamPart{Type: StreamPartTypeToolCall, ID: "call", ToolCallName: "tick", ToolCallInput: "{}"}
			if opens == 1 {
				return streamParts(call), nil
			}
			return streamParts(call, StreamPart{Type: StreamPartTypeFinish, FinishReason: FinishReasonToolCalls}), nil
		}}
		tool := NewAgentTool("tick", "", func(context.Context, struct{}, ToolCall) (ToolResponse, error) {
			executions++
			return NewTextResponse("ok"), nil
		})
		_, err := NewAgent(language, WithTools(tool), WithMaxRetries(1)).Stream(t.Context(), AgentStreamCall{
			Prompt: "hello", StopWhen: []StopCondition{StepCountIs(1)},
			OnModelMessages: func([]Message) error {
				checkpoints++
				return nil
			},
		})
		if err != nil || opens != 2 || executions != 1 || checkpoints != 1 {
			t.Fatalf("got error=%v opens=%d executions=%d checkpoints=%d", err, opens, executions, checkpoints)
		}
	})
}
