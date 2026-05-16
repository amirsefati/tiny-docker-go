//go:build linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const currentExecutable = "/proc/self/exe"
const stopGracePeriod = 2 * time.Second
const stopPollInterval = 100 * time.Millisecond

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

	network, err := newNetworkConfig(request.Network)
	if err != nil {
		return fmt.Errorf("configure network mode: %w", err)
	}
	request.Network = network.Mode()

	childArgs := []string{"child"}
	if request.Hostname != "" {
		childArgs = append(childArgs, "--hostname", request.Hostname)
	}
	if request.RootFS != "" {
		childArgs = append(childArgs, "--rootfs", request.RootFS)
	}
	if request.Network != "" {
		childArgs = append(childArgs, "--net", request.Network)
	}
	childArgs = append(childArgs, request.Command)
	childArgs = append(childArgs, request.Args...)

	container, err := s.store.NewContainer(request)
	if err != nil {
		return fmt.Errorf("create container metadata: %w", err)
	}

	if err := s.store.Save(container); err != nil {
		return fmt.Errorf("persist container metadata: %w", err)
	}

	logFile, err := os.OpenFile(s.store.LogPath(container.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open container log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.CommandContext(ctx, currentExecutable, childArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, logFile)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | network.CloneFlags(),
	}

	cgroup, err := newCgroupManager(container.ID)
	if err != nil {
		container.Status = StatusStopped
		if saveErr := s.store.Save(container); saveErr != nil {
			return fmt.Errorf("create container cgroup: %w (metadata update failed: %v)", err, saveErr)
		}

		return fmt.Errorf("create container cgroup: %w", err)
	}
	defer func() {
		_ = cgroup.Cleanup()
	}()

	if err := cgroup.ApplyMemoryLimit(request.Memory); err != nil {
		container.Status = StatusStopped
		if saveErr := s.store.Save(container); saveErr != nil {
			return fmt.Errorf("configure container cgroup: %w (metadata update failed: %v)", err, saveErr)
		}

		return fmt.Errorf("configure container cgroup: %w", err)
	}

	if err := cmd.Start(); err != nil {
		container.Status = StatusStopped
		if saveErr := s.store.Save(container); saveErr != nil {
			return fmt.Errorf("start container init: %w (metadata update failed: %v)", err, saveErr)
		}
		return fmt.Errorf("start container init: %w", err)
	}

	if err := cgroup.AddProcess(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		container.Status = StatusStopped
		if saveErr := s.store.Save(container); saveErr != nil {
			return fmt.Errorf("attach container to cgroup: %w (metadata update failed: %v)", err, saveErr)
		}

		return fmt.Errorf("attach container to cgroup: %w", err)
	}

	container.Status = StatusRunning
	container.PID = cmd.Process.Pid
	if err := s.store.Save(container); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("save running container metadata: %w", err)
	}

	waitErr := cmd.Wait()

	latestContainer, err := s.store.Load(container.ID)
	if err == nil {
		container = latestContainer
	}

	if container.Status != StatusStopped {
		container.Status = StatusExited
	}
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

	network, err := newNetworkConfig(request.Network)
	if err != nil {
		return fmt.Errorf("configure child network mode: %w", err)
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

	configurator := network.Configurator()
	if configurator != nil {
		if err := configurator.Setup(); err != nil {
			return fmt.Errorf("setup container network: %w", err)
		}
	}

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
			ID:        container.ID,
			Status:    string(container.Status),
			PID:       container.PID,
			CreatedAt: container.CreatedAt,
			Command:   container.Command,
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

	container.Status = StatusExited
	container.PID = 0

	return container
}

func (s *LocalService) Logs(_ context.Context, id string) (string, error) {
	if _, err := s.store.Load(id); err != nil {
		return "", fmt.Errorf("load container metadata: %w", err)
	}

	data, err := os.ReadFile(s.store.LogPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", fmt.Errorf("read container log file: %w", err)
	}

	return string(data), nil
}

func (s *LocalService) FollowLogs(ctx context.Context, id string, output io.Writer) error {
	if _, err := s.store.Load(id); err != nil {
		return fmt.Errorf("load container metadata: %w", err)
	}

	logFile, err := os.OpenFile(s.store.LogPath(id), os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open container log file: %w", err)
	}
	defer logFile.Close()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var offset int64

	for {
		bytesWritten, err := logFile.Seek(offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("seek container log file: %w", err)
		}
		offset = bytesWritten

		copied, err := io.Copy(output, logFile)
		offset += copied
		if err != nil {
			return fmt.Errorf("stream container log file: %w", err)
		}

		container, err := s.store.Load(id)
		if err != nil {
			return fmt.Errorf("refresh container metadata: %w", err)
		}

		container = refreshContainerStatus(container)
		if err := s.store.Save(container); err != nil {
			return fmt.Errorf("refresh followed container metadata: %w", err)
		}

		if container.Status != StatusRunning && copied == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *LocalService) Stop(ctx context.Context, id string) error {
	container, err := s.store.Load(id)
	if err != nil {
		return fmt.Errorf("load container metadata: %w", err)
	}

	container = refreshContainerStatus(container)
	if err := s.store.Save(container); err != nil {
		return fmt.Errorf("refresh container metadata: %w", err)
	}

	if container.Status != StatusRunning || container.PID <= 0 {
		return fmt.Errorf("container is not running (status: %s)", container.Status)
	}

	if err := syscall.Kill(container.PID, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			container.Status = StatusExited
			container.PID = 0
			if saveErr := s.store.Save(container); saveErr != nil {
				return fmt.Errorf("mark already-exited container: %w", saveErr)
			}
			return nil
		}

		return fmt.Errorf("send SIGTERM: %w", err)
	}

	stopped, err := waitForProcessExit(ctx, container.PID, stopGracePeriod)
	if err != nil {
		return err
	}

	if !stopped {
		if err := syscall.Kill(container.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("send SIGKILL: %w", err)
		}

		stopped, err = waitForProcessExit(ctx, container.PID, stopGracePeriod)
		if err != nil {
			return err
		}
		if !stopped {
			return errors.New("process did not exit after SIGKILL")
		}
	}

	container.Status = StatusStopped
	container.PID = 0
	if err := s.store.Save(container); err != nil {
		return fmt.Errorf("save stopped container metadata: %w", err)
	}

	return nil
}

func waitForProcessExit(ctx context.Context, pid int, timeout time.Duration) (bool, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()

	for {
		if err := syscall.Kill(pid, 0); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return true, nil
			}

			return false, fmt.Errorf("check process state: %w", err)
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, nil
		case <-ticker.C:
		}
	}
}
