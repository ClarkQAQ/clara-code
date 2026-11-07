package main

import (
	"context"
	"fmt"
	"log"

	"clara-code/examples/internal/demo"
	"clara-code/pkg/core"
	modelapi "clara-code/pkg/model"
)

func main() {
	type input struct {
		Name string `json:"name"`
	}
	hello := modelapi.NewAgentTool("hello", "Greet the named person.", func(_ context.Context, in input, _ modelapi.ToolCall) (modelapi.ToolResponse, error) {
		return modelapi.NewTextResponse("Hello " + in.Name), nil
	})
	model := demo.Model{Reply: func(_ context.Context, call modelapi.Call) (modelapi.Response, error) {
		for _, msg := range call.Prompt {
			for _, part := range msg.Content {
				if result, ok := modelapi.AsMessagePart[modelapi.ToolResultPart](part); ok {
					text, _ := modelapi.AsToolResultOutputType[modelapi.ToolResultOutputContentText](result.Output)
					return demo.Text(text.Text), nil
				}
			}
		}
		return modelapi.Response{Content: modelapi.ResponseContent{modelapi.ToolCallContent{
			ToolCallID: "greeting", ToolName: "hello", Input: `{"name":"Clara Core"}`,
		}}, FinishReason: modelapi.FinishReasonToolCalls}, nil
	}}
	result, err := core.NewAgent(model, core.WithTools(hello)).Generate(context.Background(), core.AgentCall{Prompt: "Use hello"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Response.Content.Text())
}
