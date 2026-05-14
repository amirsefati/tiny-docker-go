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

type LocalService struct {
	store *MetadataStore
}

func NewService() Service {
	return &LocalService{
		store: NewMetadataStore(defaultContainersRoot),
	}
}

func (s *LocalService) Run(ctx context.Context, request RunRequest) error {
	if request.Command == "" {
		return errors.New("command is required")
	}

	childArgs := []string{"child"}
	if request.Hostname != "" {
		childArgs = append(childArgs, "--hostname", request.Hostname)
	}
	if request.RootFS != "" {
		childArgs = append(childArgs, "--rootfs", request.RootFS)
	}
	childArgs = append(childArgs, request.Command)
	childArgs = append(childArgs, request.Args...)

	container, err := s.store.NewContainer(request)
	if err != nil {
		return fmt.Errorf("create container metadata: %w", err)
	}

	cmd := exec.CommandContext(ctx, currentExecutable, childArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start container init: %w", err)
	}

	container.PID = cmd.Process.Pid
	if err := s.store.Save(container); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("save running container metadata: %w", err)
	}

	waitErr := cmd.Wait()

	container.Status = StatusStopped
	container.PID = 0

	if err := s.store.Save(container); err != nil {
		if waitErr != nil {
			return fmt.Errorf("container exited with error (%v) and metadata update failed: %w", waitErr, err)
		}

		return fmt.Errorf("save stopped container metadata: %w", err)
	}

	if waitErr != nil {
		return fmt.Errorf("execute container init: %w", waitErr)
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

	if request.RootFS != "" {
		if err := syscall.Chroot(request.RootFS); err != nil {
			return fmt.Errorf("chroot into %q: %w", request.RootFS, err)
		}
	}

	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("change working directory: %w", err)
	}

	if err := os.MkdirAll("/proc", 0o555); err != nil {
		return fmt.Errorf("ensure /proc mount point: %w", err)
	}

	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("mount proc: %w", err)
	}
	defer func() {
		_ = syscall.Unmount("/proc", 0)
	}()

	commandPath, err := exec.LookPath(request.Command)
	if err != nil {
		return fmt.Errorf("resolve command %q: %w", request.Command, err)
	}

	commandArgs := append([]string{request.Command}, request.Args...)
	cmd := exec.Command(commandPath, request.Args...)
	cmd.Args = commandArgs
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run command %q: %w", request.Command, err)
	}

	return nil
}

func (s *LocalService) List(context.Context) ([]ProcessInfo, error) {
	containers, err := s.store.List()
	if err != nil {
		return nil, fmt.Errorf("load containers: %w", err)
	}

	processes := make([]ProcessInfo, 0, len(containers))
	for _, container := range containers {
		container = refreshContainerStatus(container)
		if err := s.store.Save(container); err != nil {
			return nil, fmt.Errorf("refresh container metadata %q: %w", container.ID, err)
		}

		processes = append(processes, ProcessInfo{
			ID:      container.ID,
			Status:  string(container.Status),
			PID:     container.PID,
			Command: container.Command,
		})
	}

	return processes, nil
}

func refreshContainerStatus(container ContainerConfig) ContainerConfig {
	if container.Status != StatusRunning || container.PID <= 0 {
		return container
	}

	if err := syscall.Kill(container.PID, 0); err == nil {
		return container
	}

	container.Status = StatusStopped
	container.PID = 0

	return container
}

func (s *LocalService) Logs(context.Context, string) (string, error) {
	return "log storage is not implemented yet", nil
}

func (s *LocalService) Stop(context.Context, string) error {
	return errors.New("stop is not implemented yet")
}
