package demo

import (
	"context"

	"clara-code/pkg/model"
)

func Static(text string) func(context.Context, model.Call) (model.Response, error) {
	return func(context.Context, model.Call) (model.Response, error) {
		return Text(text), nil
	}
}

func ToolThenText(name, input string) func(context.Context, model.Call) (model.Response, error) {
	return func(_ context.Context, call model.Call) (model.Response, error) {
		for _, msg := range call.Prompt {
			for _, part := range msg.Content {
				if item, ok := model.AsMessagePart[model.ToolResultPart](part); ok {
					return Text(toolOutput(item.Output)), nil
				}
			}
		}
		return model.Response{Content: model.ResponseContent{model.ToolCallContent{
			ToolCallID: "demo_call", ToolName: name, Input: input,
		}}, FinishReason: model.FinishReasonToolCalls}, nil
	}
}
