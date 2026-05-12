# tiny-docker-go

`tiny-docker-go` is a learning project for building a small, Docker-like container runtime in Go.

The goal is to grow this project in clear stages:

1. Start with a clean CLI and runtime shape.
2. Execute processes directly on the host.
3. Add Linux isolation primitives such as namespaces and cgroups.
4. Add metadata, logging, and lifecycle management.
5. Explore images, filesystems, and networking later.

## Day 1 scope

This first version focuses on project structure and command boundaries.

Implemented today:

- Go module setup
- CLI commands: `run`, `ps`, `stop`, `logs`
- Internal package layout for future runtime work
- A simple `run` command that executes a normal Linux command without isolation
- Consistent error handling

Not implemented yet:

- Process isolation
- Background containers
- Persistent container metadata
- Log storage
- Real stop semantics

## Project layout

```text
tiny-docker-go/
├── cmd/
│   └── tiny-docker-go/
│       └── main.go
├── internal/
│   ├── app/
│   │   └── app.go
│   ├── cli/
│   │   ├── command.go
│   │   ├── logs.go
│   │   ├── ps.go
│   │   ├── run.go
│   │   └── stop.go
│   └── runtime/
│       ├── local_runner.go
│       └── service.go
├── go.mod
└── README.md
```

## Quick start

Build:

```bash
go build ./...
```

Run a command:

```bash
go run ./cmd/tiny-docker-go run echo hello
```

Show placeholders for future lifecycle commands:

```bash
go run ./cmd/tiny-docker-go ps
go run ./cmd/tiny-docker-go logs demo
go run ./cmd/tiny-docker-go stop demo
```

## Design notes

- `cmd/` contains only the entrypoint.
- `internal/app` wires the CLI to runtime services.
- `internal/cli` owns argument parsing and command behavior.
- `internal/runtime` holds process execution and future container lifecycle logic.

This keeps the early version simple while giving us a place to add:

- process metadata stores
- namespace and cgroup setup
- background execution
- log persistence
- networking

## Next steps

Good Day 2 directions:

- add a container ID and basic metadata model
- store process state on disk
- capture logs to files
- support detached execution
- begin Linux namespace experiments
