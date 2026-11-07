package demo

import (
	"context"
	"errors"
	"strings"
	"time"

	"clara-code/pkg/models"
)

// Model is a deterministic model for examples and integration checks.
type Model struct {
	Reply func(context.Context, models.Call) (models.Response, error)
}

func (Model) Provider() string { return "fixture" }
func (Model) Model() string    { return "demo" }

func (m Model) Generate(ctx context.Context, call models.Call) (*models.Response, error) {
	if m.Reply != nil {
		response, err := m.Reply(ctx, call)
		return &response, err
	}
	var lines []string
	for _, msg := range call.Prompt {
		if msg.Role == models.MessageRoleUser {
			for _, part := range msg.Content {
				if text, ok := models.AsMessagePart[models.TextPart](part); ok && !strings.HasPrefix(text.Text, "# Current workspace state") {
					lines = append(lines, text.Text)
				}
			}
		}
	}
	last := ""
	if len(lines) > 0 {
		last = lines[len(lines)-1]
	}
	if strings.Contains(last, "CANCEL_SLOW_TURN") {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	response := Text("Demo response: " + last)
	return &response, nil
}

func (m Model) Stream(ctx context.Context, call models.Call) (models.StreamResponse, error) {
	response, err := m.Generate(ctx, call)
	if err != nil {
		return nil, err
	}
	return func(yield func(models.StreamPart) bool) {
		for _, content := range response.Content {
			switch item := content.(type) {
			case models.TextContent:
				if !yield(models.StreamPart{Type: models.StreamPartTypeTextStart, ID: "text"}) ||
					!yield(models.StreamPart{Type: models.StreamPartTypeTextDelta, ID: "text", Delta: item.Text}) ||
					!yield(models.StreamPart{Type: models.StreamPartTypeTextEnd, ID: "text"}) {
					return
				}
			case models.ToolCallContent:
				if !yield(models.StreamPart{Type: models.StreamPartTypeToolCall, ID: item.ToolCallID, ToolCallName: item.ToolName, ToolCallInput: item.Input}) {
					return
				}
			default:
				if !yield(models.StreamPart{Type: models.StreamPartTypeContent, Content: content}) {
					return
				}
			}
		}
		yield(models.StreamPart{Type: models.StreamPartTypeFinish, FinishReason: response.FinishReason, Usage: response.Usage})
	}, nil
}

func (Model) GenerateObject(context.Context, models.ObjectCall) (*models.ObjectResponse, error) {
	return nil, errors.New("fixture does not generate objects")
}

func (Model) StreamObject(context.Context, models.ObjectCall) (models.ObjectStreamResponse, error) {
	return nil, errors.New("fixture does not generate objects")
}

func Text(text string) models.Response {
	return models.Response{Content: models.ResponseContent{models.TextContent{Text: text}},
		FinishReason: models.FinishReasonStop, Usage: models.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}}
}
