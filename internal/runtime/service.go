package runtime

import "context"

type RunRequest struct {
	Hostname string
	RootFS   string
	Command  string
	Args     []string
}

type ProcessInfo struct {
	ID      string
	Status  string
	Command string
}

type Service interface {
	Run(context.Context, RunRequest) error
	List(context.Context) ([]ProcessInfo, error)
	Logs(context.Context, string) (string, error)
	Stop(context.Context, string) error
}
