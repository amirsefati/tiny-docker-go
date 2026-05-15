package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

type LogsCommand struct {
	reader ProcessReader
	stdout io.Writer
}

func (c *LogsCommand) Execute(ctx context.Context, args []string) error {
	flagSet := flag.NewFlagSet("logs", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	follow := flagSet.Bool("f", false, "follow logs")

	if err := flagSet.Parse(args); err != nil {
		return fmt.Errorf("parse logs flags: %w", err)
	}

	if len(flagSet.Args()) != 1 {
		return errors.New("logs requires exactly one container ID")
	}

	containerID := flagSet.Args()[0]
	if *follow {
		if err := c.reader.FollowLogs(ctx, containerID, c.stdout); err != nil {
			return fmt.Errorf("follow logs for %q: %w", containerID, err)
		}

		return nil
	}

	logs, err := c.reader.Logs(ctx, containerID)
	if err != nil {
		return fmt.Errorf("read logs for %q: %w", containerID, err)
	}

	_, err = io.WriteString(c.stdout, logs)
	return err
}
