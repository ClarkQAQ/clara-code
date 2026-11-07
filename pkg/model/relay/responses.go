package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"clara-code/pkg/model/relay/internal/models"
	respwire "clara-code/pkg/model/relay/internal/models/protocol/responses"
)

func (h *Handler) handleResponses(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	bodyBytes, err := readBody(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var body respwire.CreateRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	// Known fields are reconstructed from the canonical request. In particular,
	// do not forward a downstream previous_response_id as an upstream ID.
	for _, key := range []string{"model", "input", "previous_response_id", "instructions", "tools", "tool_choice", "metadata", "text",
		"temperature", "top_p", "parallel_tool_calls", "stream", "max_output_tokens", "reasoning", "prompt_cache_key", "store", "include"} {
		delete(raw, key)
	}
	body.Extra = raw
	request, err := requestFromResponses(body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if body.Model == "" {
		writeJSONError(w, http.StatusBadRequest, "model_required", "model is required")
		return
	}
	request, err = h.resolveChain(request, h.chains)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, previousNotFoundCode, err.Error())
		return
	}
	model, upstream, err := h.model(body.Model)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	request.Model = model.Info().ID
	if upstream.Protocol == ProtocolCompletions {
		if len(request.Extra) > 0 || len(request.Include) > 0 || request.ReasoningSummary != "" || request.TextVerbosity != "" {
			w.Header().Set("Clary-Conversion-Warning", "Responses-specific request options may be omitted in Chat Completions")
		}
		request.Extra = nil
	}
	if body.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)
		response, err := streamResponses(r.Context(), model, request, body, func(value map[string]any) error { return writeSSE(w, flusher, value) })
		if err == nil && response != nil {
			h.storeChain(body, request, response)
		}
		return
	}
	response, err := models.Generate(r.Context(), model, request)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	response.ID = "resp_" + newID()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responseResource(response, body)); err != nil {
		return
	}
	h.storeChain(body, request, response)
}

func (h *Handler) storeChain(body respwire.CreateRequest, request models.Request, response *models.Response) {
	store := body.Store == nil || *body.Store
	if !store {
		return
	}
	h.chains.put(response.ID, chainEntry{
		input: request.Messages, newInput: nil,
		output: response.Message, store: true,
	})
}

func writeSSE(w io.Writer, flusher http.Flusher, value map[string]any) error {
	kind, _ := value["type"].(string)
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func isPreviousNotFound(err error) bool {
	var target previousResponseError
	return errors.As(err, &target)
}
