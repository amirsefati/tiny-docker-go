package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"tiny-docker-go/internal/runtime"
)

type Runner interface {
	Run(context.Context, runtime.RunRequest) error
}

type ChildExecutor interface {
	Runner
	ChildRunner
}

type ProcessReader interface {
	List(context.Context) ([]runtime.ProcessInfo, error)
	Logs(context.Context, string) (string, error)
	FollowLogs(context.Context, string, io.Writer) error
	Stop(context.Context, string) error
}

type Command struct {
	runHandler   *RunCommand
	childHandler *ChildCommand
	psHandler    *PSCommand
	stopHandler  *StopCommand
	logsHandler  *LogsCommand
	stdout       io.Writer
	stderr       io.Writer
}

func NewCommand(service runtime.Service) *Command {
	runner, ok := service.(ChildExecutor)
	if !ok {
		panic("runtime service must implement child execution")
	}

	return &Command{
		runHandler:   &RunCommand{runner: runner},
		childHandler: &ChildCommand{runner: runner},
		psHandler:    &PSCommand{reader: service, stdout: os.Stdout},
		stopHandler:  &StopCommand{reader: service},
		logsHandler:  &LogsCommand{reader: service, stdout: os.Stdout},
		stdout:       os.Stdout,
		stderr:       os.Stderr,
	}
}

func (c *Command) Execute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return c.writeMainHelp("")
	}

	switch args[0] {
	case "run":
		return c.runHandler.Execute(ctx, args[1:])
	case "child":
		return c.childHandler.Execute(ctx, args[1:])
	case "ps":
		return c.psHandler.Execute(ctx, args[1:])
	case "stop":
		return c.stopHandler.Execute(ctx, args[1:])
	case "logs":
		return c.logsHandler.Execute(ctx, args[1:])
	case "help", "--help", "-h":
		if len(args) == 1 {
			return c.writeMainHelp("")
		}

		return c.writeCommandHelp(args[1])
	default:
		return c.usageError(fmt.Sprintf("unknown command %q", args[0]))
	}
}

func (c *Command) writeMainHelp(message string) error {
	_, err := io.WriteString(c.stdout, c.mainHelp(message))
	return err
}

func (c *Command) writeCommandHelp(name string) error {
	switch name {
	case "run":
		_, err := io.WriteString(c.stdout, runHelpText())
		return err
	case "ps":
		_, err := io.WriteString(c.stdout, psHelpText())
		return err
	case "stop":
		_, err := io.WriteString(c.stdout, stopHelpText())
		return err
	case "logs":
		_, err := io.WriteString(c.stdout, logsHelpText())
		return err
	default:
		return c.usageError(fmt.Sprintf("unknown command %q", name))
	}
}

func (c *Command) usageError(message string) error {
	return fmt.Errorf(c.mainHelp(message))
}

func (c *Command) mainHelp(message string) string {
	var builder strings.Builder

	if message != "" {
		builder.WriteString(message)
		builder.WriteString("\n\n")
	}

	builder.WriteString("tiny-docker-go is a small container runtime built in Go.\n\n")
	builder.WriteString("Usage:\n")
	builder.WriteString("  tiny-docker-go <command> [options]\n\n")
	builder.WriteString("Commands:\n")
	builder.WriteString("  run    Start a containerized command with namespaces, chroot, and optional cgroup/network setup\n")
	builder.WriteString("  ps     List tracked containers from local metadata\n")
	builder.WriteString("  stop   Stop a running container by ID\n")
	builder.WriteString("  logs   Print or follow saved container logs\n")
	builder.WriteString("  help   Show general or command-specific help\n\n")
	builder.WriteString("Examples:\n")
	builder.WriteString("  tiny-docker-go run --rootfs ./rootfs/alpine /bin/sh\n")
	builder.WriteString("  tiny-docker-go run --memory 128m --net isolated --rootfs ./rootfs/alpine /bin/sh\n")
	builder.WriteString("  tiny-docker-go ps\n")
	builder.WriteString("  tiny-docker-go logs -f <container-id>\n")
	builder.WriteString("  tiny-docker-go stop <container-id>\n\n")
	builder.WriteString("Use \"tiny-docker-go help <command>\" for details on a specific command.\n")

	return builder.String()
}
