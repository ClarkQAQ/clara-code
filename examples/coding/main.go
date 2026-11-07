package main

import (
	"context"
	"log"
	"os"

	"clara-code/examples/internal/demo"
)

func main() {
	if err := demo.CodingLoop(context.Background(), os.Stdout); err != nil {
		log.Fatal(err)
	}
}
