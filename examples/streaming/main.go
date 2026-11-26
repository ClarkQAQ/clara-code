package main

import (
	"context"
	"fmt"
	"log"

	"clara-code/examples/internal/demo"
	"clara-code/pkg/core"
)

func main() {
	// stdout is append-only. An early disconnect can retry until the first
	// text delta is printed. Replaceable previews can also set OnStreamReset.
	_, err := core.NewAgent(demo.Model{}, core.WithMaxRetries(3)).Stream(context.Background(), core.AgentStreamCall{
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
