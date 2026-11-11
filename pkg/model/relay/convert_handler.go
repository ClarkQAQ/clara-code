package relay

import (
	"net/http"
)

// Converter exposes the existing protocol conversion to callers that manage routing.
// The caller resolves continuation IDs before handing the request to the converter.
type Converter struct{}

func (Converter) ServeHTTP(w http.ResponseWriter, r *http.Request, alias string, upstream Upstream) {
	handler := Handler{
		config: Config{Upstreams: map[string]Upstream{alias: upstream}, Default: alias, StrictModels: true},
		chains: newChainStore(1),
	}
	switch r.URL.Path {
	case "/v1/responses":
		handler.handleResponses(w, r)
	case "/v1/responses/compact":
		handler.handleCompact(w, r)
	case "/v1/chat/completions":
		handler.handleCompletions(w, r)
	default:
		http.NotFound(w, r)
	}
}
