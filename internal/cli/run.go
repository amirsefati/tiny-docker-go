package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

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
	rootfs := flagSet.String("rootfs", "", "path to container root filesystem")
	memory := flagSet.String("memory", "", "memory limit, for example 128m")
	network := flagSet.String("net", "isolated", "network mode: isolated or none")

	if err := flagSet.Parse(args); err != nil {
		return runtime.RunRequest{}, fmt.Errorf("parse run flags: %w", err)
	}

	resolvedRootFS := *rootfs
	if resolvedRootFS != "" {
		absRootFS, err := filepath.Abs(resolvedRootFS)
		if err != nil {
			return runtime.RunRequest{}, fmt.Errorf("resolve rootfs path: %w", err)
		}
		resolvedRootFS = absRootFS
	}

	remaining := flagSet.Args()
	if len(remaining) == 0 {
		return runtime.RunRequest{}, errors.New("run requires a command")
	}

	return runtime.RunRequest{
		Hostname: *hostname,
		RootFS:   resolvedRootFS,
		Memory:   *memory,
		Network:  *network,
		Command:  remaining[0],
		Args:     remaining[1:],
	}, nil
}
