package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
)

type LogsCommand struct {
	reader ProcessReader
	stdout io.Writer
}

func (c *LogsCommand) Execute(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("logs requires exactly one container ID")
	}

	logs, err := c.reader.Logs(ctx, args[0])
	if err != nil {
		return fmt.Errorf("read logs for %q: %w", args[0], err)
	}

	_, err = fmt.Fprintln(c.stdout, logs)
	return err
}
