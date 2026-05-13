# tiny-docker-go

`tiny-docker-go` is a learning project for building a small, Docker-like container runtime in Go.

The goal is to grow this project in clear stages:

1. Start with a clean CLI and runtime shape.
2. Execute processes directly on the host.
3. Add Linux isolation primitives such as namespaces and cgroups.
4. Add metadata, logging, and lifecycle management.
5. Explore images, filesystems, and networking later.

## Day 2 scope

This version keeps the Day 1 CLI shape and adds the first real Linux isolation primitives to `run`.

Implemented today:

- `run --hostname <name>` flag support
- UTS namespace setup for container-local hostnames
- PID namespace setup so container processes get their own PID tree
- Mount namespace setup so `/proc` can be mounted inside the container view
- Parent/child process model using `/proc/self/exe`
- Linux-only runtime implementation with a clear non-Linux fallback error

Still not implemented:

- Filesystem root isolation with `chroot` or `pivot_root`
- cgroups for resource limits
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
│   │   ├── child.go
│   │   ├── command.go
│   │   ├── logs.go
│   │   ├── ps.go
│   │   ├── run.go
│   │   └── stop.go
│   └── runtime/
│       ├── service_linux.go
│       ├── service_unsupported.go
│       └── service.go
├── go.mod
└── README.md
```

## Quick start

Build for your current OS:

```bash
go build ./...
```

On non-Linux hosts, the binary still builds, but `run` returns a clear "Linux only" error.

Build a Linux binary:

```bash
GOOS=linux GOARCH=amd64 go build -o tiny-docker ./cmd/tiny-docker-go
```

Run an isolated container command on Linux as root:

```bash
sudo ./tiny-docker run --hostname test-container /bin/sh
```

Inside that shell, you can inspect the namespaces:

```bash
hostname
ps
```

Show placeholders for future lifecycle commands:

```bash
go run ./cmd/tiny-docker-go ps
go run ./cmd/tiny-docker-go logs demo
go run ./cmd/tiny-docker-go stop demo
```

## How namespaces work

The new `run` flow uses two stages:

1. The parent process handles CLI parsing and spawns `/proc/self/exe child ...`.
2. That child starts in fresh Linux namespaces, performs setup, and then replaces itself with the target command.

Why `/proc/self/exe`?

- It points at the current executable.
- It lets the parent re-run the same binary in a special internal mode.
- That internal mode is a simple way to separate "create namespaces" from "run the container command".

What each namespace does here:

- UTS namespace:
  Lets the container see its own hostname. Calling `sethostname` changes the hostname only inside that namespace.
- PID namespace:
  Gives the container its own process ID tree. The command you run becomes PID 1 inside the container namespace.
- Mount namespace:
  Gives the container its own mount table. That lets us mount a fresh `/proc` without affecting the host.

Why mount `/proc` again?

- `/proc` reflects the current PID namespace.
- After entering a new PID namespace, mounting `proc` inside the new mount namespace makes tools like `ps` show container-local processes instead of host processes.

## Design notes

- `cmd/` contains only the entrypoint.
- `internal/app` wires the CLI to runtime services.
- `internal/cli` owns public command parsing plus the internal `child` entrypoint.
- `internal/runtime` holds Linux namespace setup and process execution details.

This keeps the early version simple while giving us a place to add:

- process metadata stores
- namespace and cgroup setup
- background execution
- log persistence
- networking

## Next steps

Good Day 3 directions:

- add a container ID and basic metadata model
- store process state on disk
- capture logs to files
- support detached execution
- add root filesystem isolation
