# tiny-docker-go

`tiny-docker-go` is a small container runtime written in Go to learn how Docker-like systems are built from Linux primitives. It can start a process in new namespaces, `chroot` into a root filesystem, apply cgroup v2 memory limits, persist container metadata, stream logs, stop containers, and provide simple bridge-based networking.

This is a teaching project, not a production container engine. The focus is clarity, incremental implementation, and understanding the control flow from CLI to runtime.

## Features

- Linux container execution with UTS, PID, mount, and network namespaces
- `chroot`-based root filesystem isolation
- cgroup v2 memory limits with `--memory`
- container metadata and logs under `/var/lib/tiny-docker/containers`
- `ps`, `logs`, `logs -f`, and `stop` lifecycle commands
- bridge networking with a host bridge, veth pair, and NAT
- `isolated` and `none` network modes
- non-Linux fallback that still builds and returns clear runtime errors

## Architecture

### High-level flow

```text
tiny-docker-go CLI
  -> internal/cli
  -> internal/app
  -> internal/runtime service
  -> metadata store creates container record
  -> parent process re-execs /proc/self/exe as "child"
  -> child enters Linux namespaces
  -> hostname / mount / network / chroot setup
  -> target command runs inside the container context
  -> logs stream to terminal + container.log
  -> metadata updates to running / exited / stopped
```

### Component map

```text
tiny-docker-go/
├── cmd/tiny-docker-go/main.go        # program entrypoint
├── internal/app/app.go               # app wiring
├── internal/cli/                     # command parsing and help text
│   ├── command.go
│   ├── run.go
│   ├── ps.go
│   ├── logs.go
│   ├── stop.go
│   └── child.go
└── internal/runtime/                 # container runtime implementation
    ├── service_linux.go              # main Linux execution path
    ├── service_unsupported.go        # non-Linux behavior
    ├── network_linux.go              # bridge, veth, namespace network setup
    ├── cgroup_linux.go               # cgroup v2 memory limits
    ├── metadata_store.go             # config/log persistence
    └── service.go                    # shared runtime types
```

### Network path

```text
container process
  -> eth0 in container netns
  -> veth peer
  -> td0 bridge on host
  -> host routing + iptables MASQUERADE
  -> external network
```

## Installation

### Requirements

- Go 1.22+
- Linux for `run` support
- root privileges for namespaces, mounts, networking, and cgroups
- `ip` from `iproute2`
- `iptables` for outbound NAT in `isolated` mode
- cgroup v2 mounted at `/sys/fs/cgroup`
- a root filesystem to run inside, such as Alpine unpacked locally

### Build

Build for the current machine:

```bash
go build ./...
```

Build the Linux binary explicitly:

```bash
GOOS=linux GOARCH=amd64 go build -o tiny-docker-go ./cmd/tiny-docker-go
```

On macOS or Windows, the project still builds, but `run` returns a Linux-only error by design.

### Prepare a root filesystem

One simple workflow is to unpack Alpine into `./rootfs/alpine` on a Linux host:

```bash
mkdir -p rootfs/alpine
curl -L https://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/x86_64/alpine-minirootfs-latest-x86_64.tar.gz \
  | sudo tar -xz -C rootfs/alpine
```

## Usage

### Show help

```bash
./tiny-docker-go help
./tiny-docker-go help run
```

### Start a shell in a container

```bash
sudo ./tiny-docker-go run --rootfs ./rootfs/alpine /bin/sh
```

### Set a hostname

```bash
sudo ./tiny-docker-go run --hostname demo --rootfs ./rootfs/alpine /bin/sh
```

### Run with a memory limit

```bash
sudo ./tiny-docker-go run --memory 128m --rootfs ./rootfs/alpine /bin/sh
```

### Run with isolated networking

```bash
sudo ./tiny-docker-go run --net isolated --rootfs ./rootfs/alpine /bin/sh
```

### Run with loopback only

```bash
sudo ./tiny-docker-go run --net none --rootfs ./rootfs/alpine /bin/sh
```

### List tracked containers

```bash
sudo ./tiny-docker-go ps
```

### Read or follow logs

```bash
sudo ./tiny-docker-go logs <container-id>
sudo ./tiny-docker-go logs -f <container-id>
```

### Stop a container

```bash
sudo ./tiny-docker-go stop <container-id>
```

## Metadata

Each container gets a generated ID and a directory:

```text
/var/lib/tiny-docker/containers/<id>/
├── config.json
└── container.log
```

`config.json` currently stores:

- `id`
- `command`
- `hostname`
- `rootfs`
- `memory_limit`
- `network_mode`
- `ip_address`
- `status`
- `created_at`
- `pid`

## Known Limitations

- Linux only for container execution
- requires root instead of rootless execution
- uses `chroot`, not `pivot_root` or overlay filesystems
- no image format, image pull, or layered filesystem support
- no bind mounts, named volumes, or writable snapshot management
- no port publishing from host to container
- no background or detached containers
- no DNS management inside the container rootfs
- limited resource controls beyond memory
- no seccomp, capabilities dropping, AppArmor, SELinux, or user namespaces
- no TTY/interactive terminal management beyond basic stdio wiring
- metadata is local file storage only and not crash-resilient like production runtimes

## Roadmap

### Near-term

- detached containers and a `start` / `rm` lifecycle
- port publishing such as `-p 8080:80`
- bind mounts and writable container filesystems
- better error messages and validation around Linux host prerequisites
- richer `ps` output with network and memory fields

### Advanced

- `pivot_root`-based filesystem setup
- overlay filesystem support
- user namespaces and rootless mode experiments
- seccomp profiles and capability dropping
- CPU and PID cgroup controls
- minimal image format and local image cache
- embedded DNS and `/etc/resolv.conf` management
- container checkpointing or snapshot experiments

## Suggested Next Advanced Features

If you want to keep pushing this into a stronger systems project, these are the highest-value next steps:

1. Add detached containers plus persistent lifecycle commands.
2. Replace `chroot` with a safer mount-tree flow using bind mounts and `pivot_root`.
3. Add port publishing and explicit cleanup for NAT and forwarding rules.
4. Introduce bind mounts and a writable layer so containers can host realistic workloads.
5. Add security controls: user namespaces, capability dropping, and a small seccomp profile.
6. Expand cgroups beyond memory to CPU, PIDs, and I/O controls.
7. Add rootfs/image tooling so the runtime can create or fetch runnable filesystems itself.
8. Add automated integration tests on Linux for run, stop, logs, memory, and networking.

## Resume Bullet

Built a Docker-like container runtime in Go using Linux namespaces, `chroot`, cgroup v2 memory controls, bridge networking, and metadata-backed lifecycle commands (`run`, `ps`, `logs`, `stop`) to explore core container internals end to end.

## LinkedIn / GitHub Project Description

Built `tiny-docker-go`, a lightweight container runtime in Go that launches processes in isolated Linux namespaces, applies cgroup memory limits, manages logs and metadata, and connects containers through a custom bridge/veth/NAT network stack. The project is designed as a hands-on deep dive into how container runtimes work under the hood.
