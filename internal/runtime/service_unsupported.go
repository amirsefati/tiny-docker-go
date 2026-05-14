//go:build !linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"runtime"
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
			ID:      container.ID,
			Status:  string(container.Status),
			PID:     container.PID,
			Command: container.Command,
		})
	}

	return processes, nil
}

func (s *LocalService) Logs(context.Context, string) (string, error) {
	return "log storage is not implemented yet", nil
}

func (s *LocalService) Stop(context.Context, string) error {
	return errors.New("stop is not implemented yet")
}
