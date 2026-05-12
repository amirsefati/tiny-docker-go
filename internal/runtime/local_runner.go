package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

type LocalService struct{}

func NewLocalService() *LocalService {
	return &LocalService{}
}

func (s *LocalService) Run(ctx context.Context, request RunRequest) error {
	if request.Command == "" {
		return errors.New("command is required")
	}

	cmd := exec.CommandContext(ctx, request.Command, request.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execute %q: %w", request.Command, err)
	}

	return nil
}

func (s *LocalService) List(context.Context) ([]ProcessInfo, error) {
	return []ProcessInfo{}, nil
}

func (s *LocalService) Logs(context.Context, string) (string, error) {
	return "log storage is not implemented yet", nil
}

func (s *LocalService) Stop(context.Context, string) error {
	return errors.New("stop is not implemented yet")
}
