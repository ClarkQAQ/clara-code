package prompt

import (
	"encoding/json"
	"fmt"
	"strings"

	"clara-code/pkg/model"
)

// Estimate reports serialized transcript bytes and approximate model-visible tokens.
type Estimate struct {
	Bytes  int
	Tokens int64
}

// Measure estimates one token per four visible UTF-8 bytes, and 1844 per image.
// Bytes keeps the serialized transcript size with media replaced by descriptions.
func Measure(system string, messages []model.Message, tools []model.ToolInfo) (Estimate, error) {
	body, err := json.Marshal([]any{system, messagesWithMediaDescriptions(messages), tools})
	if err != nil {
		return Estimate{}, err
	}
	tokens := tokensForBytes(len(system))
	for _, message := range messages {
		visibleBytes := 0
		for _, part := range message.Content {
			visibleBytes += visiblePartBytes(part)
		}
		tokens += tokensForBytes(visibleBytes)
	}
	for _, tool := range tools {
		schema := tool.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": tool.Parameters, "required": tool.Required}
		}
		encoded, err := json.Marshal(schema)
		if err != nil {
			return Estimate{}, err
		}
		tokens += tokensForBytes(len(tool.Name) + len(tool.Description) + len(encoded))
	}
	return Estimate{Bytes: len(body), Tokens: tokens}, nil
}

func tokensForBytes(n int) int64 {
	return int64(n/4) + int64((n%4+3)/4)
}

const imageEstimateBytes = 7373

func visiblePartBytes(part model.MessagePart) int {
	if text, ok := model.AsMessagePart[model.TextPart](part); ok {
		return len(text.Text)
	}
	if reasoning, ok := model.AsMessagePart[model.ReasoningPart](part); ok {
		if reasoning.Encrypted != nil && reasoning.ItemID != "" {
			// Discount base64 expansion and the encrypted item's fixed overhead.
			return max(0, len(*reasoning.Encrypted)*3/4-650)
		}
		return len(reasoning.Text)
	}
	if file, ok := model.AsMessagePart[model.FilePart](part); ok {
		if strings.HasPrefix(file.MediaType, "image/") {
			return imageEstimateBytes
		}
		return len(file.Data) + len(file.Filename)
	}
	if call, ok := model.AsMessagePart[model.ToolCallPart](part); ok {
		return len(call.ToolName) + len(call.Input)
	}
	if result, ok := model.AsMessagePart[model.ToolResultPart](part); ok {
		size := len(result.ToolCallID)
		if text, ok := model.AsToolResultOutputType[model.ToolResultOutputContentText](result.Output); ok {
			return size + len(text.Text)
		}
		if failure, ok := model.AsToolResultOutputType[model.ToolResultOutputContentError](result.Output); ok && failure.Error != nil {
			return size + len(failure.Error.Error())
		}
		if media, ok := model.AsToolResultOutputType[model.ToolResultOutputContentMedia](result.Output); ok {
			size += len(media.Text)
			if strings.HasPrefix(media.MediaType, "image/") {
				return size + imageEstimateBytes
			}
			return size + len(media.Data)*3/4
		}
		return size
	}
	return 0
}

func messagesWithMediaDescriptions(messages []model.Message) []model.Message {
	out := make([]model.Message, len(messages))
	for i, message := range messages {
		out[i] = message
		out[i].Content = make([]model.MessagePart, len(message.Content))
		for j, part := range message.Content {
			if file, ok := model.AsMessagePart[model.FilePart](part); ok {
				part = model.TextPart{Text: fmt.Sprintf("[Attachment: %s (%s)]", file.Filename, file.MediaType)}
			}
			if result, ok := model.AsMessagePart[model.ToolResultPart](part); ok {
				if media, ok := model.AsToolResultOutputType[model.ToolResultOutputContentMedia](result.Output); ok {
					result.Output = model.ToolResultOutputContentText{Text: "[Media: " + media.MediaType + "] " + media.Text}
					part = result
				}
			}
			out[i].Content[j] = part
		}
	}
	return out
}
