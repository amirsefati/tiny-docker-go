//go:build linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const currentExecutable = "/proc/self/exe"

type LocalService struct{}

func NewService() Service {
	return &LocalService{}
}

func (s *LocalService) Run(ctx context.Context, request RunRequest) error {
	if request.Command == "" {
		return errors.New("command is required")
	}

	childArgs := []string{"child"}
	if request.Hostname != "" {
		childArgs = append(childArgs, "--hostname", request.Hostname)
	}
	childArgs = append(childArgs, request.Command)
	childArgs = append(childArgs, request.Args...)

	cmd := exec.CommandContext(ctx, currentExecutable, childArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execute container init: %w", err)
	}

	return nil
}

func (s *LocalService) RunChild(_ context.Context, request RunRequest) error {
	if request.Command == "" {
		return errors.New("command is required")
	}

	if request.Hostname != "" {
		if err := syscall.Sethostname([]byte(request.Hostname)); err != nil {
			return fmt.Errorf("set hostname: %w", err)
		}
	}

	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mounts private: %w", err)
	}

	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("mount proc: %w", err)
	}

	commandPath, err := exec.LookPath(request.Command)
	if err != nil {
		return fmt.Errorf("resolve command %q: %w", request.Command, err)
	}

	commandArgs := append([]string{request.Command}, request.Args...)
	if err := syscall.Exec(commandPath, commandArgs, os.Environ()); err != nil {
		return fmt.Errorf("exec command %q: %w", request.Command, err)
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
