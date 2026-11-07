package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientRequestShape(t *testing.T) {
	t.Parallel()

	var (
		gotPath   string
		gotAuth   string
		gotHeader string
		gotBody   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotHeader = r.Header.Get("X-Test")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}]}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL+"/v1"),
		WithAPIKey("secret"),
		WithHeader("X-Test", "yes"),
	)
	completion, err := client.ChatCompletions(t.Context(), ChatCompletionNewParams{
		Model:    "gpt-4o",
		Messages: []ChatCompletionMessageParamUnion{UserMessage("hi")},
	})
	require.NoError(t, err)
	require.Equal(t, "chatcmpl-1", completion.ID)
	require.Equal(t, "hi", completion.Choices[0].Message.Content)
	require.Equal(t, "/v1/chat/completions", gotPath)
	require.Equal(t, "Bearer secret", gotAuth)
	require.Equal(t, "yes", gotHeader)
	require.Equal(t, "gpt-4o", gotBody["model"])
	require.NotContains(t, gotBody, "stream")
}

func TestClientStreamSetsStreamFlag(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"he\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	stream := client.ChatCompletionsStream(t.Context(), ChatCompletionNewParams{Model: "gpt-4o"})
	var chunks []ChatCompletionChunk
	for stream.Next() {
		chunks = append(chunks, stream.Current())
	}
	require.NoError(t, stream.Err())
	require.Len(t, chunks, 1)
	require.Equal(t, "he", chunks[0].Choices[0].Delta.Content)
	require.Equal(t, true, gotBody["stream"])
}

func TestClientAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithAPIKey("nope"))
	_, err := client.Responses(t.Context(), ResponseNewParams{Model: "gpt-5"})

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	require.Equal(t, "bad key", apiErr.Message)
	require.Equal(t, "invalid_api_key", apiErr.Code)
	require.Contains(t, string(apiErr.DumpRequest(true)), "responses")
	require.Contains(t, string(apiErr.DumpResponse(true)), "bad key")
}

func TestClientRetries(t *testing.T) {
	t.Parallel()

	t.Run("disabled by default", func(t *testing.T) {
		t.Parallel()

		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewClient(WithBaseURL(server.URL))
		_, err := client.Responses(t.Context(), ResponseNewParams{Model: "gpt-5"})
		require.Error(t, err)
		require.Equal(t, int32(1), atomic.LoadInt32(&attempts))
	})

	t.Run("retries transient failures", func(t *testing.T) {
		t.Parallel()

		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt := atomic.AddInt32(&attempts, 1)
			if attempt < 3 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			body, _ := io.ReadAll(r.Body)
			require.Contains(t, string(body), "gpt-5")
			_, _ = io.WriteString(w, `{"id":"resp-1","model":"gpt-5"}`)
		}))
		defer server.Close()

		client := NewClient(WithBaseURL(server.URL), WithMaxRetries(3))
		response, err := client.Responses(t.Context(), ResponseNewParams{Model: "gpt-5"})
		require.NoError(t, err)
		require.Equal(t, "resp-1", response.ID)
		require.Equal(t, int32(3), atomic.LoadInt32(&attempts))
	})
}

func TestClientMiddlewareAndResponseCapture(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"chatcmpl-2"}`)
	}))
	defer server.Close()

	var (
		order    []string
		captured *http.Response
	)
	middleware := func(name string) Middleware {
		return func(request *http.Request, next MiddlewareNext) (*http.Response, error) {
			order = append(order, name)
			request.Header.Set("X-Middleware-"+name, "1")
			response, err := next(request)
			order = append(order, name+"-response")
			return response, err
		}
	}

	client := NewClient(
		WithBaseURL(server.URL),
		WithMiddleware(middleware("first")),
	)
	_, err := client.ChatCompletions(t.Context(), ChatCompletionNewParams{Model: "gpt-4o"},
		WithMiddleware(middleware("second")),
		WithResponseInto(&captured),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second", "second-response", "first-response"}, order)
	require.NotNil(t, captured)
	require.Equal(t, http.StatusOK, captured.StatusCode)
}

func TestClientPerCallOptionsDoNotLeak(t *testing.T) {
	t.Parallel()

	var headers []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Get("X-Call"))
		_, _ = io.WriteString(w, `{"id":"chatcmpl-3"}`)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	_, err := client.ChatCompletions(t.Context(), ChatCompletionNewParams{Model: "gpt-4o"}, WithHeader("X-Call", "first"))
	require.NoError(t, err)
	_, err = client.ChatCompletions(t.Context(), ChatCompletionNewParams{Model: "gpt-4o"})
	require.NoError(t, err)

	require.Equal(t, []string{"first", ""}, headers)
}

func TestStreamErrorClassification(t *testing.T) {
	t.Parallel()

	t.Run("mid stream error event", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"error\":{\"message\":\"boom\",\"type\":\"server_error\"}}\n\n")
		}))
		defer server.Close()

		stream := NewClient(WithBaseURL(server.URL)).ResponsesStream(t.Context(), ResponseNewParams{Model: "gpt-5"})
		require.False(t, stream.Next())

		var streamErr *StreamError
		require.ErrorAs(t, stream.Err(), &streamErr)
		require.Contains(t, streamErr.Message, "boom")
		require.Contains(t, string(streamErr.Event.Data), "server_error")
	})

	t.Run("http error before stream", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"nope"}}`)
		}))
		defer server.Close()

		stream := NewClient(WithBaseURL(server.URL)).ResponsesStream(t.Context(), ResponseNewParams{Model: "gpt-5"})
		require.False(t, stream.Next())

		var apiErr *Error
		require.ErrorAs(t, stream.Err(), &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	})

	t.Run("connection failure", func(t *testing.T) {
		t.Parallel()

		client := NewClient(
			WithBaseURL("http://127.0.0.1:1"),
			WithHTTPClient(HTTPClientFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			})),
		)
		_, err := client.Responses(context.Background(), ResponseNewParams{Model: "gpt-5"})
		require.ErrorContains(t, err, "dial failed")
	})
}

func TestClientContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(WithBaseURL("http://127.0.0.1:1"))
	_, err := client.Responses(ctx, ResponseNewParams{Model: "gpt-5"})
	require.True(t, errors.Is(err, context.Canceled), "expected context cancellation, got %v", err)
}

// HTTPClientFunc adapts a function to the HTTPClient interface.
type HTTPClientFunc func(*http.Request) (*http.Response, error)

func (f HTTPClientFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func TestClientSendsAcceptHeader(t *testing.T) {
	t.Parallel()

	var accept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		_, _ = fmt.Fprint(w, `{"id":"1"}`)
	}))
	defer server.Close()

	_, err := NewClient(WithBaseURL(server.URL)).ChatCompletions(t.Context(), ChatCompletionNewParams{Model: "gpt-4o"})
	require.NoError(t, err)
	require.Equal(t, "application/json", accept)
}
