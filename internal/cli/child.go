package cli

import (
	"context"
	"fmt"

	"tiny-docker-go/internal/runtime"
)

type ChildRunner interface {
	RunChild(context.Context, runtime.RunRequest) error
}

type ChildCommand struct {
	runner ChildRunner
}

func (c *ChildCommand) Execute(ctx context.Context, args []string) error {
	request, err := parseRunRequest(args)
	if err != nil {
		return err
	}

	if err := c.runner.RunChild(ctx, request); err != nil {
		return fmt.Errorf("child command: %w", err)
	}

	return nil
}
