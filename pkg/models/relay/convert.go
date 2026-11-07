package relay

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"clara-code/pkg/models/relay/internal/message"
	"clara-code/pkg/models/relay/internal/models"
	compwire "clara-code/pkg/models/relay/internal/models/protocol/completions"
	respwire "clara-code/pkg/models/relay/internal/models/protocol/responses"
)

func requestFromResponses(body respwire.CreateRequest) (models.Request, error) {
	messages, err := parseResponsesInput(body.Input)
	if err != nil {
		return models.Request{}, err
	}
	request := models.Request{
		Model: body.Model, Messages: messages, Temperature: body.Temperature,
		TopP: body.TopP, MaxOutputTokens: body.MaxOutputTokens,
		ParallelToolCalls: body.ParallelToolCalls, Store: body.Store,
		PromptCacheKey: body.PromptCacheKey, Metadata: body.Metadata, Extra: body.Extra, Include: body.Include,
	}
	if body.Instructions != nil {
		request.Instructions = *body.Instructions
	}
	if body.PreviousResponseID != nil {
		request.PreviousResponseID = *body.PreviousResponseID
	}
	for _, tool := range body.Tools {
		if tool.Type != "" && tool.Type != "function" {
			return models.Request{}, fmt.Errorf("unsupported Responses tool %q", tool.Type)
		}
		request.Tools = append(request.Tools, models.ToolDefinition{
			Name: tool.Name, Description: tool.Description,
			Parameters: tool.Parameters, Strict: tool.Strict,
		})
	}
	request.ToolChoice = parseToolChoice(body.ToolChoice)
	if body.ToolChoice != nil && request.ToolChoice == nil {
		return models.Request{}, fmt.Errorf("unsupported tool_choice")
	}
	if body.Reasoning != nil {
		request.ReasoningEffort = body.Reasoning.Effort
		request.ReasoningSummary = body.Reasoning.Summary
	}
	if body.Text != nil {
		raw, _ := json.Marshal(body.Text)
		var text struct {
			Verbosity string          `json:"verbosity"`
			Format    json.RawMessage `json:"format"`
		}
		_ = json.Unmarshal(raw, &text)
		request.TextVerbosity = text.Verbosity
		request.OutputFormat = text.Format
	}
	return request, nil
}

func parseResponsesInput(input any) ([]message.Message, error) {
	if input == nil {
		return nil, nil
	}
	if text, ok := input.(string); ok {
		return []message.Message{message.UserText(text)}, nil
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("responses input must be a string or array")
	}
	var out []message.Message
	for _, item := range items {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(item, &head); err != nil {
			return nil, err
		}
		switch head.Type {
		case "message", "":
			var value struct {
				ID      string          `json:"id"`
				Role    string          `json:"role"`
				Phase   string          `json:"phase"`
				Status  string          `json:"status"`
				Content json.RawMessage `json:"content"`
			}
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, err
			}
			parts, err := parseContent(value.Content, value.Role)
			if err != nil {
				return nil, err
			}
			role := message.Role(value.Role)
			if role == "" {
				role = message.RoleUser
			}
			out = append(out, message.Message{
				ID: value.ID, Role: role, Phase: value.Phase,
				Status: message.Status(value.Status), Parts: parts,
			})
		case "function_call":
			var value respwire.FunctionCall
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, err
			}
			out = appendAssistantPart(out, message.ToolCallPart(message.ToolCall{ID: value.CallID, Name: value.Name, Arguments: value.Arguments}))
		case "function_call_output":
			var value respwire.FunctionCallOutput
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, err
			}
			raw, err := json.Marshal(value.Output)
			if err != nil {
				return nil, err
			}
			parts, err := parseContent(raw, "tool")
			if err != nil {
				return nil, err
			}
			out = append(out, message.ToolMessage(message.ToolResult{CallID: value.CallID, Content: parts}))
		case "reasoning":
			var value respwire.ReasoningItem
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, err
			}
			var text strings.Builder
			for _, summary := range value.Summary {
				text.WriteString(summary.Text)
			}
			out = appendAssistantPart(out, message.ReasoningPart(message.Reasoning{Text: text.String(), EncryptedContent: value.EncryptedContent}))
		case "compaction":
			var value respwire.ReasoningItem
			_ = json.Unmarshal(item, &value)
			out = append(out, message.UserText("[Previous compacted context]\n"+value.EncryptedContent))
		default:
			return nil, fmt.Errorf("unsupported Responses input item %q", head.Type)
		}
	}
	return out, nil
}

func appendAssistantPart(items []message.Message, part message.Part) []message.Message {
	if len(items) > 0 && items[len(items)-1].Role == message.RoleAssistant {
		items[len(items)-1].Parts = append(items[len(items)-1].Parts, part)
		return items
	}
	return append(items, message.Assistant(part))
}

func parseContent(raw json.RawMessage, role string) ([]message.Part, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []message.Part{message.Text(text)}, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	var parts []message.Part
	for _, item := range items {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(item, &head); err != nil {
			return nil, err
		}
		switch head.Type {
		case "input_text", "output_text", "text", "summary_text", "reasoning_text":
			var value respwire.OutputText
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, err
			}
			parts = append(parts, message.Text(value.Text))
		case "refusal":
			var value respwire.Refusal
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, err
			}
			parts = append(parts, message.Refusal(value.Refusal))
		case "input_image":
			var value respwire.ImageInput
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, err
			}
			image := message.Image{URL: value.ImageURL, Detail: value.Detail}
			if data, mime, ok := parseDataURL(value.ImageURL); ok {
				image.Data, image.MIME, image.URL = data, mime, ""
			}
			parts = append(parts, message.ImagePart(image))
		case "input_file":
			var value respwire.FileInput
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, err
			}
			file := message.File{URL: value.FileURL, Filename: value.Filename}
			if data, mime, ok := parseDataURL(value.FileData); ok {
				file.Data, file.MIME, file.URL = data, mime, ""
			}
			parts = append(parts, message.FilePart(file))
		}
	}
	return parts, nil
}

func parseDataURL(value string) ([]byte, string, bool) {
	if !strings.HasPrefix(value, "data:") {
		return nil, "", false
	}
	header, encoded, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !ok || !strings.HasSuffix(header, ";base64") {
		return nil, "", false
	}
	mime := strings.TrimSuffix(header, ";base64")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", false
	}
	return data, mime, true
}

func parseToolChoice(value any) *models.ToolChoice {
	if value == nil {
		return nil
	}
	if text, ok := value.(string); ok {
		return &models.ToolChoice{Mode: text}
	}
	raw, _ := json.Marshal(value)
	var specific struct {
		Type     string                `json:"type"`
		Name     string                `json:"name"`
		Function struct{ Name string } `json:"function"`
	}
	if json.Unmarshal(raw, &specific) == nil {
		if specific.Name == "" {
			specific.Name = specific.Function.Name
		}
		if specific.Name != "" {
			return &models.ToolChoice{Mode: "specific", Name: specific.Name}
		}
	}
	return nil
}

func requestFromCompletions(body compwire.Request) (models.Request, error) {
	var messages []message.Message
	for _, item := range body.Messages {
		switch item.Role {
		case "system", "developer":
			if text, ok := item.Content.(string); ok {
				messages = append(messages, message.System(text))
			}
		case "user":
			parts, err := completionContent(item.Content)
			if err != nil {
				return models.Request{}, err
			}
			messages = append(messages, message.User(parts...))
		case "assistant":
			var parts []message.Part
			if text, _ := item.Content.(string); text != "" {
				parts = append(parts, message.Text(text))
			}
			if item.ReasoningContent != "" {
				parts = append(parts, message.ReasoningPart(message.Reasoning{Text: item.ReasoningContent}))
			}
			for _, call := range item.ToolCalls {
				parts = append(parts, message.ToolCallPart(message.ToolCall{
					ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments,
				}))
			}
			messages = append(messages, message.Assistant(parts...))
		case "tool":
			parts, err := completionContent(item.Content)
			if err != nil {
				return models.Request{}, err
			}
			messages = append(messages, message.ToolMessage(message.ToolResult{CallID: item.ToolCallID, Content: parts}))
		}
	}
	request := models.Request{
		Model: body.Model, Messages: messages, Temperature: body.Temperature,
		TopP: body.TopP, MaxOutputTokens: body.MaxTokens,
		ParallelToolCalls: body.ParallelToolCalls, Extra: body.Extra,
		ReasoningEffort: body.ReasoningEffort, PromptCacheKey: body.PromptCacheKey,
	}
	for _, tool := range body.Tools {
		request.Tools = append(request.Tools, models.ToolDefinition{
			Name: tool.Function.Name, Description: tool.Function.Description,
			Parameters: tool.Function.Parameters, Strict: tool.Function.Strict,
		})
	}
	request.ToolChoice = parseToolChoice(body.ToolChoice)
	if body.ToolChoice != nil && request.ToolChoice == nil {
		return models.Request{}, fmt.Errorf("unsupported tool_choice")
	}
	if body.MaxCompletionTokens != nil {
		request.MaxOutputTokens = body.MaxCompletionTokens
	}
	if body.ResponseFormat != nil {
		raw, err := json.Marshal(body.ResponseFormat)
		if err != nil {
			return models.Request{}, err
		}
		request.OutputFormat, err = models.ResponsesFormat(raw)
		if err != nil {
			return models.Request{}, err
		}
	}
	return request, nil
}

func completionContent(value any) ([]message.Part, error) {
	if text, ok := value.(string); ok {
		return []message.Part{message.Text(text)}, nil
	}
	raw, _ := json.Marshal(value)
	var parts []compwire.ContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	out := make([]message.Part, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			out = append(out, message.Text(part.Text))
		case "image_url":
			if part.ImageURL == nil {
				continue
			}
			image := message.Image{URL: part.ImageURL.URL, Detail: part.ImageURL.Detail}
			if data, mime, ok := parseDataURL(part.ImageURL.URL); ok {
				image.Data, image.MIME, image.URL = data, mime, ""
			}
			out = append(out, message.ImagePart(image))
		}
	}
	return out, nil
}

func responseItems(msg message.Message) []any {
	output := []any{}
	var text, refusal, reasoning strings.Builder
	encrypted := ""
	for _, part := range msg.Parts {
		switch part.Type {
		case message.PartText:
			text.WriteString(part.Text)
		case message.PartRefusal:
			refusal.WriteString(part.Refusal)
		case message.PartReasoning:
			if part.Reasoning != nil {
				reasoning.WriteString(part.Reasoning.Text)
				if part.Reasoning.EncryptedContent != "" {
					encrypted = part.Reasoning.EncryptedContent
				}
			}
		case message.PartToolCall:
			if call := part.ToolCall; call != nil {
				output = append(output, map[string]any{"type": "function_call", "id": "fc_" + newID(), "call_id": call.ID, "name": call.Name, "arguments": call.Arguments, "status": "completed"})
			}
		}
	}
	if reasoning.Len() > 0 || encrypted != "" {
		item := map[string]any{"type": "reasoning", "id": "rs_" + newID(), "summary": []any{}}
		if reasoning.Len() > 0 {
			item["summary"] = []any{map[string]any{"type": "summary_text", "text": reasoning.String()}}
		}
		if encrypted != "" {
			item["encrypted_content"] = encrypted
		}
		output = append([]any{item}, output...)
	}
	if text.Len() > 0 || len(output) == 0 && refusal.Len() == 0 {
		output = append(output, assistantItem(msg.Phase, map[string]any{"type": "output_text", "text": text.String(), "annotations": []any{}, "logprobs": []any{}}))
	}
	if refusal.Len() > 0 {
		output = append(output, assistantItem(msg.Phase, map[string]any{"type": "refusal", "refusal": refusal.String()}))
	}
	return output
}

func assistantItem(phase string, part map[string]any) map[string]any {
	if phase == "" {
		phase = "final_answer"
	}
	return map[string]any{"type": "message", "id": "msg_" + newID(), "status": "completed", "role": "assistant", "phase": phase, "content": []any{part}}
}

func responseResource(response *models.Response, requested respwire.CreateRequest) map[string]any {
	output := responseItems(response.Message)
	now := timeNowUnix()
	status := "completed"
	var details any
	if response.FinishReason == models.FinishLength || response.FinishReason == models.FinishContentFilter {
		status = "incomplete"
		reason := "max_output_tokens"
		if response.FinishReason == models.FinishContentFilter {
			reason = "content_filter"
		}
		details = map[string]any{"reason": reason}
		for _, value := range output {
			item := value.(map[string]any)
			if _, ok := item["status"]; ok {
				item["status"] = "incomplete"
			}
		}
	}
	text := requested.Text
	if text == nil {
		text = map[string]any{"format": map[string]any{"type": "text"}}
	}
	reasoning := any(requested.Reasoning)
	if requested.Reasoning == nil {
		reasoning = map[string]any{"effort": nil, "summary": nil}
	}
	return map[string]any{
		"id": response.ID, "object": "response", "created_at": now, "completed_at": now, "status": status,
		"incomplete_details": details, "model": response.Model, "previous_response_id": requested.PreviousResponseID,
		"instructions": requested.Instructions, "output": output, "error": nil,
		"tools": toolsResource(requested.Tools), "tool_choice": choiceResource(requested.ToolChoice),
		"truncation": "disabled", "parallel_tool_calls": boolValue(requested.ParallelToolCalls, true),
		"text": text, "top_p": numberValue(requested.TopP, 1), "presence_penalty": 0,
		"frequency_penalty": 0, "top_logprobs": 0, "temperature": numberValue(requested.Temperature, 1),
		"reasoning": reasoning, "usage": usageResource(response.Usage),
		"max_output_tokens": requested.MaxOutputTokens, "max_tool_calls": nil, "store": boolValue(requested.Store, true),
		"background": false, "service_tier": "default", "metadata": requested.Metadata,
		"safety_identifier": nil, "prompt_cache_key": emptyStringPointer(requested.PromptCacheKey),
	}
}

func toolsResource(tools []respwire.Tool) []any {
	out := make([]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type": "function", "name": tool.Name,
			"description": nullableString(tool.Description),
			"parameters":  nullableJSON(tool.Parameters),
			"strict":      tool.Strict,
		})
	}
	return out
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	var decoded any
	if json.Unmarshal(value, &decoded) != nil {
		return nil
	}
	return decoded
}

func choiceResource(value any) any {
	if value == nil {
		return "auto"
	}
	return value
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func numberValue[T ~float64](value *T, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return float64(*value)
}

func emptyStringPointer(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func usageResource(usage models.Usage) map[string]any {
	return map[string]any{
		"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens,
		"total_tokens":          usage.TotalTokens,
		"input_tokens_details":  map[string]any{"cached_tokens": usage.CacheReadTokens},
		"output_tokens_details": map[string]any{"reasoning_tokens": usage.ReasoningTokens},
	}
}
