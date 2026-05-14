package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
)

type PSCommand struct {
	reader ProcessReader
	stdout io.Writer
}

func (c *PSCommand) Execute(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return errors.New("ps does not accept arguments")
	}

	processes, err := c.reader.List(ctx)
	if err != nil {
		return fmt.Errorf("list processes: %w", err)
	}

	if len(processes) == 0 {
		_, err = fmt.Fprintln(c.stdout, "no containers tracked yet")
		return err
	}

	_, err = fmt.Fprintln(c.stdout, "ID\tSTATUS\tPID\tCOMMAND")
	if err != nil {
		return err
	}

	for _, process := range processes {
		if _, err := fmt.Fprintf(c.stdout, "%s\t%s\t%d\t%s\n", process.ID, process.Status, process.PID, process.Command); err != nil {
			return err
		}
	}

	return nil
}
