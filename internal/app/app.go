package app

import (
	"context"

	"tiny-docker-go/internal/cli"
	"tiny-docker-go/internal/runtime"
)

type App struct {
	command *cli.Command
}

func New(service runtime.Service) *App {
	return &App{
		command: cli.NewCommand(service),
	}
}

func (a *App) Run(ctx context.Context, args []string) error {
	return a.command.Execute(ctx, args)
}
