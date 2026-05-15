package runtime

import (
	"context"
	"io"
	"time"
)

type ContainerStatus string

const (
	StatusCreated ContainerStatus = "created"
	StatusRunning ContainerStatus = "running"
	StatusStopped ContainerStatus = "stopped"
	StatusExited  ContainerStatus = "exited"
)

type RunRequest struct {
	Hostname string
	RootFS   string
	Command  string
	Args     []string
}

type ProcessInfo struct {
	ID        string
	Status    string
	PID       int
	CreatedAt time.Time
	Command   string
}

type ContainerConfig struct {
	ID        string          `json:"id"`
	Command   string          `json:"command"`
	Hostname  string          `json:"hostname"`
	RootFS    string          `json:"rootfs"`
	Status    ContainerStatus `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	PID       int             `json:"pid"`
}

type Service interface {
	Run(context.Context, RunRequest) error
	List(context.Context) ([]ProcessInfo, error)
	Logs(context.Context, string) (string, error)
	FollowLogs(context.Context, string, io.Writer) error
	Stop(context.Context, string) error
}
