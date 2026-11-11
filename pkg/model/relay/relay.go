// Package relay provides model routing and Responses/Completions protocol conversion.
package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"clara-code/pkg/model/relay/internal/message"
	"clara-code/pkg/model/relay/internal/models"
	"clara-code/pkg/model/relay/internal/models/completions"
	"clara-code/pkg/model/relay/internal/models/responses"
	"clara-code/pkg/util/id"
)

// Protocol is an upstream wire protocol.
type Protocol string

const (
	ProtocolResponses   Protocol = "responses"
	ProtocolCompletions Protocol = "completions"
)

// Upstream configures one model endpoint.
type Upstream struct {
	BaseURL    string
	APIKey     string
	Protocol   Protocol
	Model      string
	Headers    http.Header
	HTTPClient interface {
		Do(*http.Request) (*http.Response, error)
	}
}

// Config configures a Relay handler.
type Config struct {
	Upstreams map[string]Upstream
	Default   string
	// StrictModels rejects explicit aliases missing from Upstreams.
	StrictModels bool
	Authorize    func(*http.Request) error
}

// Handler routes Responses and Chat Completions requests.
type Handler struct {
	config Config
	chains *chainStore
}

// New creates a Relay handler.
func New(config Config) (*Handler, error) {
	if len(config.Upstreams) == 0 {
		return nil, errors.New("relay: at least one upstream is required")
	}
	for name, upstream := range config.Upstreams {
		if upstream.BaseURL == "" {
			return nil, fmt.Errorf("relay: upstream %s has no base URL", name)
		}
		if upstream.Protocol != ProtocolResponses && upstream.Protocol != ProtocolCompletions {
			return nil, fmt.Errorf("relay: upstream %s has invalid protocol %q", name, upstream.Protocol)
		}
	}
	config.Upstreams = maps.Clone(config.Upstreams)
	for name, upstream := range config.Upstreams {
		upstream.Headers = upstream.Headers.Clone()
		config.Upstreams[name] = upstream
	}
	if config.Default == "" {
		config.Default = "default"
		if _, ok := config.Upstreams[config.Default]; !ok {
			config.Default = slices.Sorted(maps.Keys(config.Upstreams))[0]
		}
	}
	if _, ok := config.Upstreams[config.Default]; !ok {
		return nil, fmt.Errorf("unknown default upstream %q", config.Default)
	}
	return &Handler{config: config, chains: newChainStore(512)}, nil
}

// ServeHTTP implements the Relay HTTP surface.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.config.Authorize != nil {
		if err := h.config.Authorize(r); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", err.Error())
			return
		}
	}
	switch r.URL.Path {
	case "/healthz", "/v1/healthz":
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	case "/v1/responses":
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			h.serveWebSocket(w, r)
			return
		}
		h.handleResponses(w, r)
	case "/v1/responses/compact":
		h.handleCompact(w, r)
	case "/v1/chat/completions":
		h.handleCompletions(w, r)
	default:
		writeJSONError(w, http.StatusNotFound, "not_found", "unknown Relay path")
	}
}

func (h *Handler) model(modelName string) (models.Model, Upstream, error) {
	upstream, ok := h.config.Upstreams[modelName]
	if !ok && (!h.config.StrictModels || modelName == "") {
		upstream, ok = h.config.Upstreams[h.config.Default]
	}
	if !ok {
		return nil, Upstream{}, fmt.Errorf("relay: no upstream for model %q", modelName)
	}
	modelID := upstream.Model
	if modelID == "" {
		modelID = modelName
	}
	switch upstream.Protocol {
	case ProtocolResponses:
		var doer responses.HTTPDoer
		if upstream.HTTPClient != nil {
			doer = upstream.HTTPClient
		}
		adapter, err := responses.New(responses.Config{
			BaseURL: upstream.BaseURL, APIKey: upstream.APIKey, Model: modelID,
			Headers: upstream.Headers, HTTPClient: doer,
		})
		return adapter, upstream, err
	case ProtocolCompletions:
		var doer completions.HTTPDoer
		if upstream.HTTPClient != nil {
			doer = upstream.HTTPClient
		}
		adapter, err := completions.New(completions.Config{
			BaseURL: upstream.BaseURL, APIKey: upstream.APIKey, Model: modelID,
			Headers: upstream.Headers, HTTPClient: doer,
		})
		return adapter, upstream, err
	default:
		return nil, upstream, errors.New("relay: invalid upstream protocol")
	}
}

func (h *Handler) resolveChain(
	request models.Request,
	local *chainStore,
) (models.Request, error) {
	if request.PreviousResponseID == "" {
		return request, nil
	}
	chain, ok := local.get(request.PreviousResponseID)
	if !ok {
		chain, ok = h.chains.get(request.PreviousResponseID)
	}
	if !ok {
		return models.Request{}, previousNotFound(request.PreviousResponseID)
	}
	if err := validateContinuation(chain.output, request.Messages); err != nil {
		return models.Request{}, err
	}
	next := request.Messages
	request.Messages = append(append([]message.Message{}, chain.input...), chain.output)
	request.Messages = append(request.Messages, chain.newInput...)
	request.Messages = append(request.Messages, next...)
	request.PreviousResponseID = ""
	return request, nil
}

func validateContinuation(previousOutput message.Message, next []message.Message) error {
	callIDs := map[string]bool{}
	for _, call := range previousOutput.ToolCalls() {
		callIDs[call.ID] = true
	}
	for _, msg := range next {
		for _, part := range msg.Parts {
			if part.Type == message.PartToolResult && part.ToolResult != nil && !callIDs[part.ToolResult.CallID] {
				return fmt.Errorf("function_call_output %q has no matching tool call", part.ToolResult.CallID)
			}
		}
	}
	return nil
}

type chainEntry struct {
	input    []message.Message
	newInput []message.Message
	output   message.Message
	store    bool
	bytes    int
}

type chainStore struct {
	mu      sync.Mutex
	max     int
	entries map[string]chainEntry
	order   []string
	bytes   int
}

func newChainStore(max int) *chainStore {
	return &chainStore{max: max, entries: map[string]chainEntry{}}
}

func (s *chainStore) get(id string) (chainEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	return entry, ok
}

func (s *chainStore) put(id string, entry chainEntry) {
	raw, err := json.Marshal(struct {
		Input, NewInput []message.Message
		Output          message.Message
	}{entry.input, entry.newInput, entry.output})
	if err != nil || len(raw) > 32<<20 {
		return
	}
	entry.bytes = len(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.entries[id]; ok {
		s.bytes -= old.bytes
	} else {
		s.order = append(s.order, id)
	}
	s.entries[id] = entry
	s.bytes += entry.bytes
	for len(s.order) > s.max || s.bytes > 64<<20 {
		old := s.order[0]
		s.order = s.order[1:]
		s.bytes -= s.entries[old].bytes
		delete(s.entries, old)
	}
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, (32<<20)+1))
	if len(data) > 32<<20 {
		return nil, errors.New("request exceeds the size limit")
	}
	return data, err
}

func decodeBody(r *http.Request, target any) error {
	raw, err := readBody(r)
	if err != nil {
		return err
	}
	if !json.Valid(raw) {
		return errors.New("invalid JSON request")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type": "invalid_request_error", "code": code, "message": message,
		},
	})
}

type previousResponseError struct{ id string }

func (e previousResponseError) Error() string { return "previous response not found: " + e.id }

func previousNotFound(id string) error { return previousResponseError{id: id} }

const previousNotFoundCode = "previous_response_not_found"

func newID() string {
	return id.New("")
}

func timeNowUnix() int64 { return time.Now().Unix() }
