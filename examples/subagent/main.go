package main

import (
	"context"
	"fmt"
	"log"

	"clara-code/examples/internal/demo"
	"clara-code/pkg/core"
	"clara-code/pkg/tools"
)

func main() {
	ctx := context.Background()
	manager := tools.NewTaskManager(func(context.Context, tools.TaskSpec, *tools.TaskScope) (tools.TaskSetup, error) {
		return tools.TaskSetup{Model: demo.Model{Reply: demo.Static("Child confirmed the task result.")}}, nil
	}, 4, 3)
	scope := manager.Scope(ctx, "", tools.ScopeOptions{})
	defer func() {
		if err := scope.Close(ctx); err != nil {
			log.Print(err)
		}
	}()
	engine := core.NewAgent(demo.Model{Reply: demo.ToolThenText("subagent_task", `{"task_name":"check","role":"reviewer","message":"Check the result."}`)},
		core.WithTools(tools.RunTask(scope)))
	result, err := engine.Generate(ctx, core.AgentCall{Prompt: "Delegate the review."})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Response.Content.Text())
}
