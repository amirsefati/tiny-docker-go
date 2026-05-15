//go:build !linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
)

type LocalService struct{}

func NewService() Service {
	return &LocalService{}
}

func (s *LocalService) Run(context.Context, RunRequest) error {
	return fmt.Errorf("run is only supported on Linux; current OS is %s", runtime.GOOS)
}

func (s *LocalService) RunChild(context.Context, RunRequest) error {
	return fmt.Errorf("child execution is only supported on Linux; current OS is %s", runtime.GOOS)
}

func (s *LocalService) List(context.Context) ([]ProcessInfo, error) {
	store := NewMetadataStore(defaultContainersRoot)
	containers, err := store.List()
	if err != nil {
		return nil, fmt.Errorf("load containers: %w", err)
	}

	processes := make([]ProcessInfo, 0, len(containers))
	for _, container := range containers {
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

func (s *LocalService) Logs(_ context.Context, id string) (string, error) {
	store := NewMetadataStore(defaultContainersRoot)
	if _, err := store.Load(id); err != nil {
		return "", fmt.Errorf("load container metadata: %w", err)
	}

	data, err := os.ReadFile(store.LogPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", fmt.Errorf("read container log file: %w", err)
	}

	return string(data), nil
}

func (s *LocalService) FollowLogs(ctx context.Context, id string, output io.Writer) error {
	store := NewMetadataStore(defaultContainersRoot)
	if _, err := store.Load(id); err != nil {
		return fmt.Errorf("load container metadata: %w", err)
	}

	logFile, err := os.OpenFile(store.LogPath(id), os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open container log file: %w", err)
	}
	defer logFile.Close()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var offset int64

	for {
		position, err := logFile.Seek(offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("seek container log file: %w", err)
		}
		offset = position

		copied, err := io.Copy(output, logFile)
		offset += copied
		if err != nil {
			return fmt.Errorf("stream container log file: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *LocalService) Stop(context.Context, string) error {
	return errors.New("stop is only supported on Linux")
}
