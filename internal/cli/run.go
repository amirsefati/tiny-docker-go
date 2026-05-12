package cli

import (
	"context"
	"errors"
	"fmt"

	"tiny-docker-go/internal/runtime"
)

type RunCommand struct {
	runner Runner
}

func (c *RunCommand) Execute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("run requires a command")
	}

	request := runtime.RunRequest{
		Command: args[0],
		Args:    args[1:],
	}

	if err := c.runner.Run(ctx, request); err != nil {
		return fmt.Errorf("run command: %w", err)
	}

	return nil
}
