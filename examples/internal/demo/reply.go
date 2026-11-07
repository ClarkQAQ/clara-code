package demo

import (
	"context"

	"clara-code/pkg/models"
)

func Static(text string) func(context.Context, models.Call) (models.Response, error) {
	return func(context.Context, models.Call) (models.Response, error) {
		return Text(text), nil
	}
}

func ToolThenText(name, input string) func(context.Context, models.Call) (models.Response, error) {
	return func(_ context.Context, call models.Call) (models.Response, error) {
		for _, msg := range call.Prompt {
			for _, part := range msg.Content {
				if item, ok := models.AsMessagePart[models.ToolResultPart](part); ok {
					return Text(toolOutput(item.Output)), nil
				}
			}
		}
		return models.Response{Content: models.ResponseContent{models.ToolCallContent{
			ToolCallID: "demo_call", ToolName: name, Input: input,
		}}, FinishReason: models.FinishReasonToolCalls}, nil
	}
}
