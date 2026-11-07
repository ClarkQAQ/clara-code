package main

import (
	"context"
	"fmt"
	"log"

	"clara-code/examples/internal/demo"
	"clara-code/pkg/core"
	"clara-code/pkg/tools"
	"clara-code/pkg/tools/fs"
	"clara-code/pkg/tools/fs/memory"
)

func main() {
	ctx := context.Background()
	storage := memory.New()
	if _, err := storage.WriteFile(ctx, "/hello.txt", []byte("Hello from memory."), fs.WriteOptions{}); err != nil {
		log.Fatal(err)
	}
	read := tools.NewFiles(storage, nil).Read()
	engine := core.NewAgent(demo.Model{Reply: demo.ToolThenText("read", `{"path":"hello.txt"}`)}, core.WithTools(read))
	result, err := engine.Generate(ctx, core.AgentCall{Prompt: "Read hello.txt"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Response.Content.Text())
}
