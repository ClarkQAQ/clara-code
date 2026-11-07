package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	respwire "clara-code/pkg/model/relay/internal/models/protocol/responses"
)

func (h *Handler) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	connection.SetReadLimit(16 << 20)
	defer connection.CloseNow()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Minute)
	defer cancel()
	local := newChainStore(64)
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, connection, &raw); err != nil {
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Type != "response.create" {
			_ = wsjson.Write(ctx, connection, map[string]any{
				"type": "error", "status": 400,
				"error": map[string]any{"code": "invalid_request", "message": "expected response.create"},
			})
			continue
		}
		var body respwire.CreateRequest
		if err := json.Unmarshal(raw, &body); err != nil {
			_ = wsjson.Write(ctx, connection, map[string]any{
				"type": "error", "status": 400,
				"error": map[string]any{"code": "invalid_request", "message": err.Error()},
			})
			continue
		}
		body.Extra = map[string]json.RawMessage{}
		request, err := requestFromResponses(body)
		if err != nil {
			_ = wsjson.Write(ctx, connection, map[string]any{
				"type": "error", "status": 400,
				"error": map[string]any{"code": "invalid_request", "message": err.Error()},
			})
			continue
		}
		request, err = h.resolveChain(request, local)
		if err != nil {
			code := "invalid_request"
			if isPreviousNotFound(err) {
				code = previousNotFoundCode
			}
			_ = wsjson.Write(ctx, connection, map[string]any{
				"type": "error", "status": 400,
				"error": map[string]any{"code": code, "message": err.Error()},
			})
			if body.PreviousResponseID != nil {
				local.delete(*body.PreviousResponseID)
			}
			continue
		}
		model, _, err := h.model(body.Model)
		if err != nil {
			_ = wsjson.Write(ctx, connection, map[string]any{
				"type": "error", "status": 502,
				"error": map[string]any{"code": "upstream_error", "message": err.Error()},
			})
			continue
		}
		request.Model = model.Info().ID
		response, err := streamResponses(ctx, model, request, body, func(value map[string]any) error { return wsjson.Write(ctx, connection, value) })
		if err != nil {
			return
		}
		if response == nil {
			if body.PreviousResponseID != nil {
				local.delete(*body.PreviousResponseID)
			}
			continue
		}
		store := body.Store != nil && *body.Store
		if !store {
			local.put(response.ID, chainEntry{
				input: request.Messages, newInput: nil, output: response.Message,
			})
		} else {
			h.chains.put(response.ID, chainEntry{
				input: request.Messages, output: response.Message, store: true,
			})
		}
	}
}

func (s *chainStore) delete(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entries[id]; ok {
		s.bytes -= entry.bytes
		delete(s.entries, id)
		for index, key := range s.order {
			if key == id {
				s.order = append(s.order[:index], s.order[index+1:]...)
				break
			}
		}
	}
}
