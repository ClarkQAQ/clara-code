package demo

import (
	"context"
	"errors"
	"strings"
	"time"

	"clara-code/pkg/model"
)

// Model is a deterministic model for examples and integration checks.
type Model struct {
	Reply func(context.Context, model.Call) (model.Response, error)
}

func (Model) Provider() string { return "fixture" }
func (Model) Model() string    { return "demo" }

func (m Model) Generate(ctx context.Context, call model.Call) (*model.Response, error) {
	if m.Reply != nil {
		response, err := m.Reply(ctx, call)
		return &response, err
	}
	var lines []string
	for _, msg := range call.Prompt {
		if msg.Role == model.MessageRoleUser {
			for _, part := range msg.Content {
				if text, ok := model.AsMessagePart[model.TextPart](part); ok && !strings.HasPrefix(text.Text, "# Current workspace state") {
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

func (m Model) Stream(ctx context.Context, call model.Call) (model.StreamResponse, error) {
	response, err := m.Generate(ctx, call)
	if err != nil {
		return nil, err
	}
	return func(yield func(model.StreamPart) bool) {
		for _, content := range response.Content {
			switch item := content.(type) {
			case model.TextContent:
				if !yield(model.StreamPart{Type: model.StreamPartTypeTextStart, ID: "text"}) ||
					!yield(model.StreamPart{Type: model.StreamPartTypeTextDelta, ID: "text", Delta: item.Text}) ||
					!yield(model.StreamPart{Type: model.StreamPartTypeTextEnd, ID: "text"}) {
					return
				}
			case model.ToolCallContent:
				if !yield(model.StreamPart{Type: model.StreamPartTypeToolCall, ID: item.ToolCallID, ToolCallName: item.ToolName, ToolCallInput: item.Input}) {
					return
				}
			default:
				if !yield(model.StreamPart{Type: model.StreamPartTypeContent, Content: content}) {
					return
				}
			}
		}
		yield(model.StreamPart{Type: model.StreamPartTypeFinish, FinishReason: response.FinishReason, Usage: response.Usage})
	}, nil
}

func (Model) GenerateObject(context.Context, model.ObjectCall) (*model.ObjectResponse, error) {
	return nil, errors.New("fixture does not generate objects")
}

func (Model) StreamObject(context.Context, model.ObjectCall) (model.ObjectStreamResponse, error) {
	return nil, errors.New("fixture does not generate objects")
}

func Text(text string) model.Response {
	return model.Response{Content: model.ResponseContent{model.TextContent{Text: text}},
		FinishReason: model.FinishReasonStop, Usage: model.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}}
}
