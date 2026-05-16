package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"tiny-docker-go/internal/runtime"
)

type RunCommand struct {
	runner Runner
}

func (c *RunCommand) Execute(ctx context.Context, args []string) error {
	if isHelpRequest(args) {
		_, err := io.WriteString(os.Stdout, runHelpText())
		return err
	}

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

func runHelpText() string {
	return `Usage:
  tiny-docker-go run [--hostname name] [--rootfs path] [--memory limit] [--net mode] <command> [args...]

Options:
  --hostname string   Set the container hostname inside the UTS namespace
  --rootfs path       Chroot into a local root filesystem before running the command
  --memory limit      Apply a cgroup v2 memory limit such as 64m, 128m, or 1g
  --net mode          Network mode: isolated or none (default: isolated)

Examples:
  tiny-docker-go run --rootfs ./rootfs/alpine /bin/sh
  tiny-docker-go run --hostname demo --rootfs ./rootfs/alpine /bin/sh
  tiny-docker-go run --memory 128m --net isolated --rootfs ./rootfs/alpine /bin/sh

Notes:
  This command requires Linux privileges for namespaces, mounts, networking, and cgroups.
`
}
