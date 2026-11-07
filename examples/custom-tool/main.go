package main

import (
	"context"
	"fmt"
	"log"

	"clara-code/examples/internal/demo"
	"clara-code/pkg/core"
	"clara-code/pkg/models"
)

func main() {
	type input struct {
		Name string `json:"name"`
	}
	hello := models.NewAgentTool("hello", "Greet the named person.", func(_ context.Context, in input, _ models.ToolCall) (models.ToolResponse, error) {
		return models.NewTextResponse("Hello " + in.Name), nil
	})
	model := demo.Model{Reply: func(_ context.Context, call models.Call) (models.Response, error) {
		for _, msg := range call.Prompt {
			for _, part := range msg.Content {
				if result, ok := models.AsMessagePart[models.ToolResultPart](part); ok {
					text, _ := models.AsToolResultOutputType[models.ToolResultOutputContentText](result.Output)
					return demo.Text(text.Text), nil
				}
			}
		}
		return models.Response{Content: models.ResponseContent{models.ToolCallContent{
			ToolCallID: "greeting", ToolName: "hello", Input: `{"name":"Clara Core"}`,
		}}, FinishReason: models.FinishReasonToolCalls}, nil
	}}
	result, err := core.NewAgent(model, core.WithTools(hello)).Generate(context.Background(), core.AgentCall{Prompt: "Use hello"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Response.Content.Text())
}
