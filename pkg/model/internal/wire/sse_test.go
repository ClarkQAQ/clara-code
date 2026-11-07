package openai

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func decodeEvents(t *testing.T, payload string) []Event {
	t.Helper()

	response := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   io.NopCloser(strings.NewReader(payload)),
	}
	decoder := newDecoder(response)
	t.Cleanup(func() { _ = decoder.Close() })

	var events []Event
	for decoder.Next() {
		events = append(events, decoder.Event())
	}
	require.NoError(t, decoder.Err())
	return events
}

func TestDecoderParsesEvents(t *testing.T) {
	t.Parallel()

	t.Run("multiple events", func(t *testing.T) {
		t.Parallel()

		events := decodeEvents(t, "event: message\ndata: {\"a\":1}\n\ndata: [DONE]\n\n")
		require.Len(t, events, 2)
		require.Equal(t, "message", events[0].Type)
		require.JSONEq(t, `{"a":1}`, string(events[0].Data))
		require.Equal(t, "[DONE]", strings.TrimSpace(string(events[1].Data)))
	})

	t.Run("multi line data and comments", func(t *testing.T) {
		t.Parallel()

		events := decodeEvents(t, ": keep-alive\ndata: {\"a\":\ndata: 1}\n\n")
		require.Len(t, events, 1)
		require.JSONEq(t, `{"a":1}`, string(events[0].Data))
	})

	t.Run("carriage returns", func(t *testing.T) {
		t.Parallel()

		events := decodeEvents(t, "data: {\"a\":1}\r\n\r\n")
		require.Len(t, events, 1)
		require.JSONEq(t, `{"a":1}`, string(events[0].Data))
	})

	t.Run("no trailing separator", func(t *testing.T) {
		t.Parallel()

		require.Empty(t, decodeEvents(t, "data: {\"a\":1}"))
	})
}

func TestStreamLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("stops at done", func(t *testing.T) {
		t.Parallel()

		response := &http.Response{
			Header: http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:   io.NopCloser(strings.NewReader("data: {\"id\":\"1\"}\n\ndata: [DONE]\n\ndata: {\"id\":\"2\"}\n\n")),
		}
		stream := newStream[ChatCompletionChunk](response, nil)
		defer stream.Close()

		require.True(t, stream.Next())
		require.Equal(t, "1", stream.Current().ID)
		require.False(t, stream.Next())
		require.NoError(t, stream.Err())
	})

	t.Run("invalid payload ends the stream with an error", func(t *testing.T) {
		t.Parallel()

		response := &http.Response{
			Header: http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:   io.NopCloser(strings.NewReader("data: not-json\n\n")),
		}
		stream := newStream[ChatCompletionChunk](response, nil)
		defer stream.Close()

		require.False(t, stream.Next())
		require.Error(t, stream.Err())
	})

	t.Run("error carried in payload", func(t *testing.T) {
		t.Parallel()

		response := &http.Response{
			Header: http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:   io.NopCloser(strings.NewReader("data: {\"error\":\"nope\"}\n\n")),
		}
		stream := newStream[ChatCompletionChunk](response, nil)
		defer stream.Close()

		require.False(t, stream.Next())
		var streamErr *StreamError
		require.ErrorAs(t, stream.Err(), &streamErr)
	})

	t.Run("request error is reported", func(t *testing.T) {
		t.Parallel()

		stream := newStream[ChatCompletionChunk](nil, io.ErrUnexpectedEOF)
		defer stream.Close()

		require.False(t, stream.Next())
		require.ErrorIs(t, stream.Err(), io.ErrUnexpectedEOF)
	})
}
