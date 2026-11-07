package main

import (
	"context"
	"fmt"
	"log"

	"clara-code/examples/internal/demo"
	"clara-code/pkg/core"
)

func main() {
	_, err := core.NewAgent(demo.Model{}).Stream(context.Background(), core.AgentStreamCall{
		Prompt: "Stream this response",
		OnTextDelta: func(_ string, delta string) error {
			_, err := fmt.Print(delta)
			return err
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println()
}
