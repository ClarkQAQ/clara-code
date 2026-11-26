package prompt

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
)

func (c Compactor) summarize(ctx context.Context, m model.LanguageModel, system string, history []model.Message) (string, error) {
	history = slices.Clone(history)
	if c.Project != nil {
		history = c.Project(history)
	}
	instructions := c.Instructions
	if instructions == "" {
		instructions = Default("compact")
	}
	if !slices.ContainsFunc(history, summaryHistoryItem) {
		return "", errors.New("there is no conversation to summarize")
	}
	retry := core.RetryWithExponentialBackoffRespectingRetryHeaders[*model.Response](core.DefaultRetryOptions())
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		request := make([]model.Message, 0, len(history)+2)
		if system != "" {
			request = append(request, model.NewDeveloperMessage(system))
		}
		for _, message := range history {
			message.Metadata = nil
			request = append(request, message)
		}
		request = append(request, model.NewUserMessage(instructions))
		var overflow error
		response, err := retry(ctx, func() (*model.Response, error) {
			response, err := m.Generate(ctx, model.Call{Prompt: request})
			var provider *model.ProviderError
			if errors.As(err, &provider) && provider.IsContextTooLarge() {
				// Shrink the request before retrying, even when the provider also
				// marks the error transient.
				overflow = err
				return nil, nil
			}
			return response, err
		})
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if overflow != nil {
			history = dropOldestSummaryItem(history)
			if !slices.ContainsFunc(history, summaryHistoryItem) {
				return "", overflow
			}
			continue
		}
		if err != nil {
			return "", err
		}
		if response == nil {
			return "", errors.New("compaction returned no response")
		}
		if response.FinishReason == model.FinishReasonLength {
			return "", errors.New("compaction reached the model output token limit before completing the summary")
		}
		if response.FinishReason != model.FinishReasonStop {
			return "", fmt.Errorf("compaction did not return a complete summary (finish reason %q)", response.FinishReason)
		}
		var text []string
		for _, part := range response.Content {
			if _, ok := model.AsContentType[model.ToolCallContent](part); ok {
				return "", errors.New("compaction returned an unexpected tool call")
			}
			if item, ok := model.AsContentType[model.TextContent](part); ok && strings.TrimSpace(item.Text) != "" {
				text = append(text, item.Text)
			}
		}
		if len(text) == 0 {
			return "", errors.New("compaction returned an empty summary")
		}
		return strings.Join(text, "\n"), nil
	}
}

func summaryHistoryItem(message model.Message) bool {
	return len(message.Content) > 0 && message.Role != model.MessageRoleSystem && message.Role != model.MessageRoleDeveloper &&
		(message.Metadata == nil || message.Metadata.Kind != "context")
}

// A request that drops a call must also drop its outputs, even when parallel
// results arrive in separate messages or share a message with other output.
func dropOldestSummaryItem(history []model.Message) []model.Message {
	calls := make(map[string]bool)
	for _, part := range history[0].Content {
		if call, ok := model.AsMessagePart[model.ToolCallPart](part); ok && !call.ProviderExecuted {
			calls[call.ToolCallID] = true
		}
	}
	if len(calls) == 0 {
		return history[1:]
	}
	var out []model.Message
	for _, message := range history[1:] {
		if message.Role == model.MessageRoleTool && len(calls) > 0 {
			message.Content = slices.DeleteFunc(slices.Clone(message.Content), func(part model.MessagePart) bool {
				result, ok := model.AsMessagePart[model.ToolResultPart](part)
				if !ok || result.ProviderExecuted || !calls[result.ToolCallID] {
					return false
				}
				delete(calls, result.ToolCallID)
				return true
			})
			if len(message.Content) == 0 {
				continue
			}
		}
		out = append(out, message)
	}
	return out
}
