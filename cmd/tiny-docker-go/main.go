package main

import (
	"context"
	"fmt"
	"os"

	"tiny-docker-go/internal/app"
	"tiny-docker-go/internal/runtime"
)

func main() {
	application := app.New(runtime.NewService())

	if err := application.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
