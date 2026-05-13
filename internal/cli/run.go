package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"tiny-docker-go/internal/runtime"
)

type RunCommand struct {
	runner Runner
}

func (c *RunCommand) Execute(ctx context.Context, args []string) error {
	request, err := parseRunRequest(args)
	if err != nil {
		return err
	}

	if err := c.runner.Run(ctx, request); err != nil {
		return fmt.Errorf("run command: %w", err)
	}

	return nil
}

func parseRunRequest(args []string) (runtime.RunRequest, error) {
	flagSet := flag.NewFlagSet("run", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	hostname := flagSet.String("hostname", "", "container hostname")

	if err := flagSet.Parse(args); err != nil {
		return runtime.RunRequest{}, fmt.Errorf("parse run flags: %w", err)
	}

	remaining := flagSet.Args()
	if len(remaining) == 0 {
		return runtime.RunRequest{}, errors.New("run requires a command")
	}

	return runtime.RunRequest{
		Hostname: *hostname,
		Command:  remaining[0],
		Args:     remaining[1:],
	}, nil
}
