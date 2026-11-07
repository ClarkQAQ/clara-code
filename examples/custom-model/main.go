package main

import (
	"context"
	"fmt"
	"log"

	"clara-code/examples/internal/demo"
	"clara-code/pkg/core"
)

func main() {
	engine := core.NewAgent(demo.Model{}, core.WithPrepareStep(func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		return ctx, core.PrepareStepResult{Model: demo.Model{Reply: demo.Static("Selected a different model for this step.")}}, nil
	}))
	result, err := engine.Generate(context.Background(), core.AgentCall{Prompt: "Select a model"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Response.Content.Text())
}
