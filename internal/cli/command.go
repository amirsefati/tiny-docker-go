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
	Stop(context.Context, string) error
}

type Command struct {
	runHandler   *RunCommand
	childHandler *ChildCommand
	psHandler    *PSCommand
	stopHandler  *StopCommand
	logsHandler  *LogsCommand
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
		stderr:       os.Stderr,
	}
}

func (c *Command) Execute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return c.usageError("")
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
		return c.usageError("")
	default:
		return c.usageError(fmt.Sprintf("unknown command %q", args[0]))
	}
}

func (c *Command) usageError(message string) error {
	var builder strings.Builder

	if message != "" {
		builder.WriteString(message)
		builder.WriteString("\n\n")
	}

	builder.WriteString("usage:\n")
	builder.WriteString("  tiny-docker-go run [--hostname name] [--rootfs path] <command> [args...]\n")
	builder.WriteString("  tiny-docker-go ps\n")
	builder.WriteString("  tiny-docker-go stop <id>\n")
	builder.WriteString("  tiny-docker-go logs <id>\n")

	return fmt.Errorf(builder.String())
}
