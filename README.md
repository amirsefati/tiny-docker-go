# tiny-docker-go

`tiny-docker-go` is a learning project for building a small, Docker-like container runtime in Go.

The goal is to grow this project in clear stages:

1. Start with a clean CLI and runtime shape.
2. Execute processes directly on the host.
3. Add Linux isolation primitives such as namespaces and cgroups.
4. Add metadata, logging, and lifecycle management.
5. Explore images, filesystems, and networking later.

## Day 9 scope

This version keeps the earlier namespace, `chroot`, lifecycle, and cgroup work, and adds simple container networking through a Linux bridge, a veth pair, and NAT.

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
- stored container fields: `id`, `command`, `hostname`, `rootfs`, `memory_limit`, `network_mode`, `ip_address`, `status`, `created_at`, `pid`
- lifecycle statuses: `created`, `running`, `stopped`, `exited`
- `ps` implementation backed by saved container metadata
- `ps` output improved with cleaner columns and container creation time
- container state refresh for stale `running` entries when `ps` and `logs -f` run
- `logs <id>` implementation backed by `container.log`
- `logs -f <id>` follow support for running containers
- stdout and stderr mirrored to both the terminal and the container log file
- `stop <id>` implementation with `SIGTERM` followed by `SIGKILL` fallback
- per-container cgroup creation under `/sys/fs/cgroup/tiny-docker/<id>`
- cgroup v2 memory limits through `run --memory <value>`
- automatic PID attachment to the container cgroup after process start
- cgroup cleanup after the container exits
- `run --net isolated` and `run --net none` flag support
- network namespace setup with `CLONE_NEWNET`
- loopback interface brought up inside the container namespace
- host bridge creation as `td0`
- bridge address assignment as `10.10.0.1/24`
- host-side veth creation for isolated containers
- one side of the veth pair moved into the container network namespace and renamed to `eth0`
- container IP allocation from the `10.10.0.0/24` subnet
- container interface address assignment such as `10.10.0.2/24`
- container default route via `10.10.0.1`
- IPv4 forwarding enabled on the host
- host NAT rules added with `iptables` so containers can reach the outside network
- host veth cleanup when the container exits
- network setup separated into a small runtime helper for bridge/veth/NAT management
- Parent/child process model using `/proc/self/exe`
- Linux-only runtime implementation with a clear non-Linux fallback error

Still not implemented:

- Strong filesystem isolation with `pivot_root`, mount propagation rules, and bind-mount setup
- DNS configuration inside the container root filesystem
- Port publishing from host to container
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
│       ├── cgroup_linux.go
│       ├── metadata_store.go
│       ├── network_linux.go
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

Run with a memory limit:

```bash
sudo ./tiny-docker run --memory 128m --rootfs ./rootfs/alpine /bin/sh
```

Run with an isolated network namespace:

```bash
sudo ./tiny-docker run --net isolated --rootfs ./rootfs/alpine /bin/sh
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
- Network namespace:
  Gives the container its own network stack. Interfaces, routes, firewall state, and ports are separate from the host.

Why mount `/proc` again?

- `/proc` reflects the current PID namespace.
- After entering a new PID namespace, mounting `proc` inside the new mount namespace makes tools like `ps` show container-local processes instead of host processes.

## How Linux network namespaces work

A Linux network namespace gives a process its own private copy of the networking world.

Inside a new network namespace, the process gets its own:

- network interfaces
- routing table
- firewall state
- port bindings
- ARP and neighbor tables

That means a process inside the container can listen on port `8080` without colliding with something on host port `8080`, because those ports now live in different network namespaces.

The runtime now does two layers of setup:

1. Start the container in a fresh network namespace with `CLONE_NEWNET`.
2. Bring up the `lo` interface inside that namespace.
3. On the host, ensure a bridge called `td0` exists.
4. Give that bridge the gateway address `10.10.0.1/24`.
5. Create a veth pair, which acts like a virtual Ethernet cable.
6. Keep one end on the host and attach it to `td0`.
7. Move the other end into the container namespace and rename it to `eth0`.
8. Inside the container namespace, assign an address like `10.10.0.2/24` to `eth0`.
9. Bring `eth0` up and add a default route through `10.10.0.1`.
10. On the host, enable IPv4 forwarding and add `iptables` MASQUERADE rules.

Why bring up loopback first?

- many programs expect `127.0.0.1` to work
- local services inside the container may talk to themselves over loopback
- a brand-new network namespace usually starts with loopback present but down

So after this change, `localhost` works inside the container and the container can also send traffic toward the host bridge and then out to the internet through NAT.

## How the Day 9 networking path works

Think of the packet path like this:

```text
container process
  -> eth0 inside the container namespace
  -> veth peer
  -> td0 bridge on the host
  -> host routing + iptables MASQUERADE
  -> external network
```

Each step has a small job:

- `td0` is the container-side switch on the host. Multiple containers can later plug into the same bridge.
- the veth pair is the cable between namespaces. Packets that leave one end appear on the other end.
- `10.10.0.1/24` on `td0` is the container subnet gateway.
- `10.10.0.x/24` on the container's `eth0` gives the container an address on that subnet.
- the default route tells the container "send non-local traffic to `10.10.0.1`".
- IPv4 forwarding tells the Linux kernel it may route packets between interfaces.
- the `iptables` MASQUERADE rule rewrites the source IP from `10.10.0.x` to the host's outward-facing address so replies can come back.

Why NAT is needed here:

- the `10.10.0.0/24` subnet is private and only exists locally on the host
- outside machines do not know how to route replies back to `10.10.0.2`
- MASQUERADE makes outbound packets look like they came from the host instead
- reply packets come back to the host, and connection tracking maps them back to the container

## Networking modes right now

The project currently supports these flags:

- `--net isolated`
- `--net none`

Both modes use a fresh network namespace, but they now differ on purpose:

- `isolated` means "new network namespace plus bridge/veth/NAT connectivity"
- `none` means "new network namespace with only loopback"

Example checks inside the container:

```sh
hostname
ping -c 1 127.0.0.1
ping -c 1 1.1.1.1
ip addr show eth0
ip route
```

Expected behavior:

- `127.0.0.1` should work once loopback is up
- `eth0` should have an address like `10.10.0.2/24`
- the default route should point to `10.10.0.1`
- public IPs should work if the host has network access and `iptables` is available

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
- `memory_limit`
- `network_mode`
- `ip_address`
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

## How cgroups work

Cgroups are a Linux kernel feature for putting processes into a named group and applying resource rules to that group.

For this project, the important idea is:

- namespaces change what the container can see
- cgroups change how much of a resource the container can use

So if namespaces are about isolation, cgroups are about limits and accounting.

This Day 7 version uses cgroup v2 like this:

1. Create a directory for the container under `/sys/fs/cgroup/tiny-docker/<id>`.
2. If `--memory` was provided, write the limit into `memory.max`.
3. Start the container process and write its host PID into `cgroup.procs`.
4. Wait for the process to exit, then remove the cgroup directory.

Example:

```bash
sudo ./tiny-docker run --memory 128m --rootfs ./rootfs/alpine /bin/sh
```

That means the whole container process tree gets a memory budget of 128 MiB.

Current limitation:

- this implementation expects cgroup v2 to be available at `/sys/fs/cgroup`
- if the host only has cgroup v1, `run` will return a clear error instead of silently ignoring the limit

## Testing the memory limit

You need a Linux host with cgroup v2 enabled and enough privileges to create cgroups.

First, verify cgroup v2 is present:

```bash
test -f /sys/fs/cgroup/cgroup.controllers && echo "cgroup v2 ready"
```

Then start a shell with a small limit:

```bash
sudo ./tiny-docker run --memory 64m --rootfs ./rootfs/alpine /bin/sh
```

Inside the container shell, try to allocate more than that limit:

```sh
python3 -c 'chunks=[]; [chunks.append(bytearray(10*1024*1024)) for _ in range(20)]'
```

If `python3` is not available in the rootfs, try BusyBox tools to stress memory from another package set, or build a rootfs that includes Python.

What you should see:

- the allocation command should fail or the process should be killed by the kernel
- the container should exit rather than growing without bound

From the host, you can also inspect the cgroup value while the container is running:

```bash
cat /sys/fs/cgroup/tiny-docker/<container-id>/memory.max
```

For a `64m` run, that file should contain `67108864`.

## Design notes

- `cmd/` contains only the entrypoint.
- `internal/app` wires the CLI to runtime services.
- `internal/cli` owns public command parsing plus the internal `child` entrypoint.
- `internal/runtime` holds Linux namespace setup, process execution, cgroup logic, and the bridge/veth/NAT networking helpers.

This keeps the early version simple while giving us a place to add:

- process metadata stores
- namespace and cgroup setup
- bridge and veth networking helpers
- background execution
- networking

## Next steps

Good next directions:

- support detached execution
- improve filesystem isolation beyond basic `chroot`
- record more runtime state such as cgroup paths or exit codes
- add DNS setup such as writing `/etc/resolv.conf` inside the container rootfs
- add port forwarding from host ports into container ports
- add background container supervision and reaping
