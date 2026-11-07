package provider

import (
	yuzu "clara-code/pkg/models"
	openai "clara-code/pkg/models/internal/wire"
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithRequestOptions(t *testing.T) {
	t.Parallel()

	server := newMockServer()
	defer server.close()
	server.prepareJSONResponse(map[string]any{"content": "hi"})

	var called atomic.Bool
	provider, err := New(
		WithAPIKey("test-api-key"),
		WithBaseURL(server.server.URL),
		WithRequestOptions(
			openai.WithHeader("X-Gateway", "yes"),
			openai.WithMiddleware(func(request *http.Request, next openai.MiddlewareNext) (*http.Response, error) {
				called.Store(true)
				return next(request)
			}),
		),
	)
	require.NoError(t, err)

	model, err := provider.LanguageModel(t.Context(), "gpt-4o")
	require.NoError(t, err)
	_, err = model.Generate(context.Background(), yuzu.Call{Prompt: testPrompt})
	require.NoError(t, err)

	require.True(t, called.Load(), "middleware passed through WithRequestOptions must run")
	require.Len(t, server.calls, 1)
	require.Equal(t, "yes", server.calls[0].headers["X-Gateway"])
}
