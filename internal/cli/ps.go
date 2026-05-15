package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
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

	writer := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)

	_, err = fmt.Fprintln(writer, "ID\tSTATUS\tPID\tCREATED\tCOMMAND")
	if err != nil {
		return err
	}

	for _, process := range processes {
		pid := "-"
		if process.PID > 0 {
			pid = strconv.Itoa(process.PID)
		}

		createdAt := process.CreatedAt.Local().Format("2006-01-02 15:04:05")
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", process.ID, process.Status, pid, createdAt, process.Command); err != nil {
			return err
		}
	}

	return writer.Flush()
}
