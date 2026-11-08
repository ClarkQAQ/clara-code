package prompt

import (
	"encoding/json"
	"fmt"
	"strings"

	"clara-code/pkg/model"
)

// Estimate is a conservative heuristic, not tokenizer-accurate usage.
type Estimate struct {
	Bytes  int
	Tokens int64
}

// Measure includes tool definitions and text, reserving 8192 tokens per image
// instead of counting its base64 payload as text.
func Measure(system string, messages []model.Message, tools []model.ToolInfo) (Estimate, error) {
	text, media := summaryMessages(messages)
	body, err := json.Marshal([]any{system, text, tools})
	if err != nil {
		return Estimate{}, err
	}
	return Estimate{Bytes: len(body), Tokens: int64((len(body)+1)/2) + media*8192}, nil
}

func summaryMessages(messages []model.Message) ([]model.Message, int64) {
	out := make([]model.Message, len(messages))
	var mediaCount int64
	for i, message := range messages {
		out[i] = message
		out[i].Content = make([]model.MessagePart, len(message.Content))
		for j, part := range message.Content {
			if file, ok := model.AsMessagePart[model.FilePart](part); ok {
				part = model.TextPart{Text: fmt.Sprintf("[Attachment: %s (%s)]", file.Filename, file.MediaType)}
				if strings.HasPrefix(file.MediaType, "image/") {
					mediaCount++
				}
			}
			if result, ok := model.AsMessagePart[model.ToolResultPart](part); ok {
				if media, ok := model.AsToolResultOutputType[model.ToolResultOutputContentMedia](result.Output); ok {
					result.Output = model.ToolResultOutputContentText{Text: "[Media: " + media.MediaType + "] " + media.Text}
					part = result
					if strings.HasPrefix(media.MediaType, "image/") {
						mediaCount++
					}
				}
			}
			out[i].Content[j] = part
		}
	}
	return out, mediaCount
}
