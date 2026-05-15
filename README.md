# tiny-docker-go

`tiny-docker-go` is a learning project for building a small, Docker-like container runtime in Go.

The goal is to grow this project in clear stages:

1. Start with a clean CLI and runtime shape.
2. Execute processes directly on the host.
3. Add Linux isolation primitives such as namespaces and cgroups.
4. Add metadata, logging, and lifecycle management.
5. Explore images, filesystems, and networking later.

## Day 6 scope

This version keeps the earlier namespace and `chroot` work, and adds basic container lifecycle management.

Implemented today:

- `run --hostname <name>` flag support
- `run --rootfs <path>` flag support for a local root filesystem
- UTS namespace setup for container-local hostnames
- PID namespace setup so container processes get their own PID tree
- Mount namespace setup so `/proc` can be mounted inside the container view
- `chroot` into the selected root filesystem
- working directory change to `/` after entering the container root
- `/proc` mount cleanup after the container command exits
- generated container IDs for each `run`
- local metadata storage under `/var/lib/tiny-docker/containers/<id>/config.json`
- local log storage under `/var/lib/tiny-docker/containers/<id>/container.log`
- stored container fields: `id`, `command`, `hostname`, `rootfs`, `status`, `created_at`, `pid`
- lifecycle statuses: `created`, `running`, `stopped`, `exited`
- `ps` implementation backed by saved container metadata
- `ps` output improved with cleaner columns and container creation time
- container state refresh for stale `running` entries when `ps` and `logs -f` run
- `logs <id>` implementation backed by `container.log`
- `logs -f <id>` follow support for running containers
- stdout and stderr mirrored to both the terminal and the container log file
- `stop <id>` implementation with `SIGTERM` followed by `SIGKILL` fallback
- Parent/child process model using `/proc/self/exe`
- Linux-only runtime implementation with a clear non-Linux fallback error

Still not implemented:

- Strong filesystem isolation with `pivot_root`, mount propagation rules, and bind-mount setup
- cgroups for resource limits
- Background containers

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
│       ├── metadata_store.go
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
sudo ./tiny-docker run --hostname test-container --rootfs ./rootfs/alpine /bin/sh
```

Inside that shell, you can inspect the namespaces:

```bash
hostname
ps
```

List tracked containers:

```bash
sudo ./tiny-docker ps
```

Read saved logs:

```bash
sudo ./tiny-docker logs <container-id>
sudo ./tiny-docker logs -f <container-id>
```

Stop a running container:

```bash
sudo ./tiny-docker stop <container-id>
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

## `chroot` vs Docker

This project now uses `chroot` as a simple teaching step.

- `chroot` changes what a process sees as `/`.
- It does not build a full container filesystem model by itself.
- Real Docker typically combines mount namespaces, carefully prepared mount trees, overlay filesystems, bind mounts, `pivot_root`, cgroups, capabilities, seccomp, and more.

So in this project, `chroot` gives us a local root filesystem view, but it is not the same security boundary or filesystem isolation model that Docker provides in production.

## Metadata layout

Each container now gets a generated ID and a local directory:

```text
/var/lib/tiny-docker/containers/<id>/
├── config.json
└── container.log
```

The `config.json` file stores:

- `id`
- `command`
- `hostname`
- `rootfs`
- `status`
- `created_at`
- `pid`

This gives the runtime a simple local source of truth for `ps` and later lifecycle features.

The `container.log` file stores the combined stdout and stderr stream for each container.

Status values used now:

- `created`: metadata exists, but the container process has not been started yet
- `running`: the container init process is alive
- `stopped`: the container was explicitly stopped through the runtime
- `exited`: the container process ended on its own or was discovered to be gone later

## How Docker tracks state conceptually

Conceptually, Docker keeps a metadata record for each container outside the container process itself.

- The runtime creates a container identity and stores config plus state on disk.
- It updates that state as the container moves through lifecycle stages such as created, running, stopped, or exited.
- Commands like `docker ps` read from that metadata plus live runtime signals rather than scanning arbitrary processes and guessing.
- Lower-level runtimes such as `containerd` and `runc` handle the actual process execution, while higher layers keep the durable state model in sync.

This project now mirrors that idea in a much simpler form: one folder per container, one JSON file for config and state, and a `ps` command that reads those records.

## How container process management works

At a low level, a container here is still just a normal Linux process tree with some extra isolation.

- The runtime starts a child process in new namespaces.
- That child becomes the container's init-like process from the host's point of view.
- The host keeps the real host PID in metadata so later commands like `ps`, `logs`, and `stop` know which process to manage.
- `stop` works by sending signals to that host PID, not by reaching into the container shell directly.

Why `SIGTERM` first?

- `SIGTERM` asks the process to exit cleanly.
- Well-behaved programs can flush logs, close files, and shut down in order.
- If the process ignores the signal or gets stuck, the runtime escalates to `SIGKILL`, which the kernel enforces immediately.

Why do statuses need refreshing?

- Processes can exit without the runtime being actively watching from another command.
- Because of that, commands like `ps` refresh saved `running` containers by checking whether the recorded PID still exists.
- If the PID is gone, the runtime updates metadata so the saved state stays aligned with reality.

## Design notes

- `cmd/` contains only the entrypoint.
- `internal/app` wires the CLI to runtime services.
- `internal/cli` owns public command parsing plus the internal `child` entrypoint.
- `internal/runtime` holds Linux namespace setup and process execution details.

This keeps the early version simple while giving us a place to add:

- process metadata stores
- namespace and cgroup setup
- background execution
- networking

## Next steps

Good next directions:

- support detached execution
- improve filesystem isolation beyond basic `chroot`
- add background container supervision and reaping
