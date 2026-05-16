package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

type StopCommand struct {
	reader ProcessReader
}

func (c *StopCommand) Execute(ctx context.Context, args []string) error {
	if isHelpRequest(args) {
		_, err := io.WriteString(os.Stdout, stopHelpText())
		return err
	}

	if len(args) != 1 {
		return errors.New("stop requires exactly one container ID")
	}

	if err := c.reader.Stop(ctx, args[0]); err != nil {
		return fmt.Errorf("stop container %q: %w", args[0], err)
	}

	return nil
}

func stopHelpText() string {
	return `Usage:
  tiny-docker-go stop <container-id>

Description:
  Send SIGTERM to the tracked container process, then fall back to SIGKILL if it does not exit in time.
`
}
