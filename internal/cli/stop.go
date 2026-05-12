package cli

import (
	"context"
	"errors"
	"fmt"
)

type StopCommand struct {
	reader ProcessReader
}

func (c *StopCommand) Execute(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("stop requires exactly one container ID")
	}

	if err := c.reader.Stop(ctx, args[0]); err != nil {
		return fmt.Errorf("stop container %q: %w", args[0], err)
	}

	return nil
}
