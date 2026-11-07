package main

import (
	"context"
	"fmt"
	"log"

	"clara-code/examples/internal/demo"
	"clara-code/pkg/core"
)

func main() {
	result, err := core.NewAgent(demo.Model{}).Generate(context.Background(), core.AgentCall{Prompt: "Hello Clara Core"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Response.Content.Text())
}
