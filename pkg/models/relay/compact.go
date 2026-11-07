package relay

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"clara-code/pkg/models/relay/internal/message"
	"clara-code/pkg/models/relay/internal/models"
)

func (h *Handler) handleCompact(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model              string          `json:"model"`
		Input              json.RawMessage `json:"input"`
		Instructions       *string         `json:"instructions"`
		PreviousResponseID *string         `json:"previous_response_id"`
		PromptCacheKey     *string         `json:"prompt_cache_key"`
	}
	data, err := readBody(r)
	if err != nil || json.Unmarshal(data, &body) != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "invalid compact request")
		return
	}
	if body.Model == "" {
		writeJSONError(w, http.StatusBadRequest, "model_required", "model is required")
		return
	}
	var input any
	if len(body.Input) > 0 {
		if err := json.Unmarshal(body.Input, &input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	messages, err := parseResponsesInput(input)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	request := models.Request{
		Model: body.Model,
		Messages: append([]message.Message{
			message.System("Summarize the conversation as a compact context for later continuation. Preserve goals, constraints, decisions, completed work, open tasks, exact paths, and next steps. Output only the summary."),
		}, messages...),
	}
	if body.Instructions != nil {
		request.Instructions = *body.Instructions
	}
	if body.PromptCacheKey != nil {
		request.PromptCacheKey = *body.PromptCacheKey
	}
	if body.PreviousResponseID != nil {
		request.PreviousResponseID = *body.PreviousResponseID
		request, err = h.resolveChain(request, h.chains)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, previousNotFoundCode, err.Error())
			return
		}
	}
	model, _, err := h.model(body.Model)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	request.Model = model.Info().ID
	response, err := models.Generate(r.Context(), model, request)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	summary := response.Message.Text()
	result := map[string]any{
		"id": "cmp_" + newID(), "object": "response.compaction",
		"created_at": timeNowUnix(),
		"output": []any{
			map[string]any{
				"type": "message", "id": "msg_" + newID(), "status": "completed",
				"role": "assistant", "phase": "final_answer",
				"content": []any{map[string]any{"type": "output_text", "text": summary, "annotations": []any{}}},
			},
			map[string]any{
				"type": "compaction", "id": "cmp_item_" + newID(),
				"encrypted_content": base64.StdEncoding.EncodeToString([]byte(summary)),
			},
		},
		"usage": usageResource(response.Usage),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
