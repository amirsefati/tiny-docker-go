//go:build linux

package runtime

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	NetworkModeIsolated = "isolated"
	NetworkModeNone     = "none"

	defaultBridgeName      = "td0"
	defaultBridgeCIDR      = "10.10.0.1/24"
	defaultBridgeGatewayIP = "10.10.0.1"
	defaultContainerIfName = "eth0"
	networkSetupTimeout    = 5 * time.Second
	envBridgeName          = "TINY_DOCKER_BRIDGE_NAME"
	envBridgeCIDR          = "TINY_DOCKER_BRIDGE_CIDR"
	envGatewayIP           = "TINY_DOCKER_GATEWAY_IP"
	envContainerIP         = "TINY_DOCKER_CONTAINER_IP"
	envContainerCIDR       = "TINY_DOCKER_CONTAINER_CIDR"
	envContainerInterface  = "TINY_DOCKER_CONTAINER_IFACE"
	envHostInterface       = "TINY_DOCKER_HOST_IFACE"
)

type networkConfig struct {
	mode     string
	settings *networkSettings
}

type networkSettings struct {
	BridgeName             string
	BridgeCIDR             string
	BridgeGatewayIP        string
	ContainerIP            string
	ContainerCIDR          string
	ContainerInterfaceName string
	HostInterfaceName      string
}

type networkConfigurator interface {
	Setup() error
}

type loopbackOnlyNetworkConfigurator struct{}

type isolatedNetworkConfigurator struct {
	settings networkSettings
}

func normalizeNetworkMode(mode string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(mode))
	if normalized == "" {
		return NetworkModeIsolated, nil
	}

	switch normalized {
	case NetworkModeIsolated, NetworkModeNone:
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported network mode %q", mode)
	}
}

func newNetworkConfig(mode string, settings *networkSettings) (networkConfig, error) {
	normalized, err := normalizeNetworkMode(mode)
	if err != nil {
		return networkConfig{}, err
	}

	if normalized == NetworkModeIsolated {
		if settings == nil {
			settings, err = loadNetworkSettingsFromEnv()
			if err != nil {
				return networkConfig{}, err
			}
		}

		return networkConfig{mode: normalized, settings: settings}, nil
	}

	return networkConfig{mode: normalized}, nil
}

func (c networkConfig) CloneFlags() uintptr {
	switch c.mode {
	case NetworkModeIsolated, NetworkModeNone:
		return syscall.CLONE_NEWNET
	default:
		return 0
	}
}

func (c networkConfig) Configurator() networkConfigurator {
	switch c.mode {
	case NetworkModeIsolated:
		if c.settings == nil {
			return nil
		}
		return isolatedNetworkConfigurator{settings: *c.settings}
	case NetworkModeNone:
		return loopbackOnlyNetworkConfigurator{}
	default:
		return nil
	}
}

func (c networkConfig) Mode() string {
	return c.mode
}

func (c networkConfig) EnvironmentVariables() []string {
	if c.mode != NetworkModeIsolated || c.settings == nil {
		return nil
	}

	return []string{
		envBridgeName + "=" + c.settings.BridgeName,
		envBridgeCIDR + "=" + c.settings.BridgeCIDR,
		envGatewayIP + "=" + c.settings.BridgeGatewayIP,
		envContainerIP + "=" + c.settings.ContainerIP,
		envContainerCIDR + "=" + c.settings.ContainerCIDR,
		envContainerInterface + "=" + c.settings.ContainerInterfaceName,
		envHostInterface + "=" + c.settings.HostInterfaceName,
	}
}

func (c networkConfig) SetupHostSide(containerPID int) (func(), error) {
	if c.mode != NetworkModeIsolated || c.settings == nil {
		return func() {}, nil
	}

	return setupContainerVeth(*c.settings, containerPID)
}

func (loopbackOnlyNetworkConfigurator) Setup() error {
	if err := bringInterfaceUp("lo"); err != nil {
		return fmt.Errorf("bring up loopback interface: %w", err)
	}

	return nil
}

func (c isolatedNetworkConfigurator) Setup() error {
	if err := (loopbackOnlyNetworkConfigurator{}).Setup(); err != nil {
		return err
	}

	if err := waitForInterface(c.settings.ContainerInterfaceName, networkSetupTimeout); err != nil {
		return fmt.Errorf("wait for container interface %q: %w", c.settings.ContainerInterfaceName, err)
	}

	if err := runCommand("ip", "addr", "replace", c.settings.ContainerCIDR, "dev", c.settings.ContainerInterfaceName); err != nil {
		return fmt.Errorf("assign %s to %s: %w", c.settings.ContainerCIDR, c.settings.ContainerInterfaceName, err)
	}

	if err := bringInterfaceUp(c.settings.ContainerInterfaceName); err != nil {
		return fmt.Errorf("bring up container interface %q: %w", c.settings.ContainerInterfaceName, err)
	}

	if err := runCommand(
		"ip",
		"route",
		"replace",
		"default",
		"via",
		c.settings.BridgeGatewayIP,
		"dev",
		c.settings.ContainerInterfaceName,
	); err != nil {
		return fmt.Errorf("add default route via %s: %w", c.settings.BridgeGatewayIP, err)
	}

	return nil
}

type ifreqFlags struct {
	Name  [syscall.IFNAMSIZ]byte
	Flags uint16
	_     [24 - syscall.IFNAMSIZ - 2]byte
}

func bringInterfaceUp(name string) error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return fmt.Errorf("open ioctl socket: %w", err)
	}
	defer syscall.Close(fd)

	var req ifreqFlags
	copy(req.Name[:], name)

	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.SIOCGIFFLAGS),
		uintptr(unsafe.Pointer(&req)),
	); errno != 0 {
		return errno
	}

	req.Flags |= syscall.IFF_UP

	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.SIOCSIFFLAGS),
		uintptr(unsafe.Pointer(&req)),
	); errno != 0 {
		return errno
	}

	return nil
}

func allocateIsolatedNetworkSettings(store *MetadataStore, containerID string) (*networkSettings, error) {
	hostInterfaceName, err := newHostVethName(containerID)
	if err != nil {
		return nil, err
	}

	containerIP, containerCIDR, err := allocateContainerAddress(store)
	if err != nil {
		return nil, err
	}

	return &networkSettings{
		BridgeName:             defaultBridgeName,
		BridgeCIDR:             defaultBridgeCIDR,
		BridgeGatewayIP:        defaultBridgeGatewayIP,
		ContainerIP:            containerIP,
		ContainerCIDR:          containerCIDR,
		ContainerInterfaceName: defaultContainerIfName,
		HostInterfaceName:      hostInterfaceName,
	}, nil
}

func loadNetworkSettingsFromEnv() (*networkSettings, error) {
	settings := &networkSettings{
		BridgeName:             os.Getenv(envBridgeName),
		BridgeCIDR:             os.Getenv(envBridgeCIDR),
		BridgeGatewayIP:        os.Getenv(envGatewayIP),
		ContainerIP:            os.Getenv(envContainerIP),
		ContainerCIDR:          os.Getenv(envContainerCIDR),
		ContainerInterfaceName: os.Getenv(envContainerInterface),
		HostInterfaceName:      os.Getenv(envHostInterface),
	}

	if settings.BridgeName == "" &&
		settings.BridgeCIDR == "" &&
		settings.BridgeGatewayIP == "" &&
		settings.ContainerIP == "" &&
		settings.ContainerCIDR == "" &&
		settings.ContainerInterfaceName == "" &&
		settings.HostInterfaceName == "" {
		return nil, errors.New("missing isolated network settings in environment")
	}

	if settings.BridgeName == "" || settings.BridgeCIDR == "" || settings.BridgeGatewayIP == "" ||
		settings.ContainerIP == "" || settings.ContainerCIDR == "" || settings.ContainerInterfaceName == "" {
		return nil, errors.New("incomplete isolated network settings in environment")
	}

	return settings, nil
}

func allocateContainerAddress(store *MetadataStore) (string, string, error) {
	containers, err := store.List()
	if err != nil {
		return "", "", fmt.Errorf("list containers for ip allocation: %w", err)
	}

	used := map[string]struct{}{
		defaultBridgeGatewayIP: {},
	}

	for _, container := range containers {
		if container.NetworkMode != NetworkModeIsolated || container.IPAddress == "" {
			continue
		}

		if container.Status != StatusCreated && container.Status != StatusRunning {
			continue
		}

		used[container.IPAddress] = struct{}{}
	}

	for hostOctet := 2; hostOctet <= 254; hostOctet++ {
		ip := fmt.Sprintf("10.10.0.%d", hostOctet)
		if _, exists := used[ip]; exists {
			continue
		}

		return ip, fmt.Sprintf("%s/24", ip), nil
	}

	return "", "", errors.New("no free container addresses left in 10.10.0.0/24")
}

func newHostVethName(containerID string) (string, error) {
	const prefix = "tdvh"
	const maxNameLen = 15
	maxSuffixLen := maxNameLen - len(prefix)
	if maxSuffixLen <= 0 {
		return "", errors.New("host veth prefix is too long")
	}

	trimmedID := strings.ToLower(strings.TrimSpace(containerID))
	if trimmedID == "" {
		return "", errors.New("container id is required to name host veth")
	}

	if len(trimmedID) > maxSuffixLen {
		trimmedID = trimmedID[:maxSuffixLen]
	}

	return prefix + trimmedID, nil
}

func setupContainerVeth(settings networkSettings, containerPID int) (func(), error) {
	if err := ensureBridge(settings.BridgeName, settings.BridgeCIDR); err != nil {
		return nil, err
	}

	if err := ensureIPv4Forwarding(); err != nil {
		return nil, err
	}

	if err := ensureNATRules(settings.BridgeName, settings.BridgeCIDR); err != nil {
		return nil, err
	}

	peerName := settings.HostInterfaceName + "p"
	if len(peerName) > 15 {
		peerName = peerName[:15]
	}

	if err := runCommand("ip", "link", "add", settings.HostInterfaceName, "type", "veth", "peer", "name", peerName); err != nil {
		return nil, fmt.Errorf("create veth pair %s <-> %s: %w", settings.HostInterfaceName, peerName, err)
	}

	cleanup := func() {
		_ = runCommand("ip", "link", "delete", settings.HostInterfaceName)
	}

	if err := runCommand("ip", "link", "set", settings.HostInterfaceName, "master", settings.BridgeName); err != nil {
		cleanup()
		return nil, fmt.Errorf("attach %s to bridge %s: %w", settings.HostInterfaceName, settings.BridgeName, err)
	}

	if err := bringInterfaceUp(settings.HostInterfaceName); err != nil {
		cleanup()
		return nil, fmt.Errorf("bring up host veth %s: %w", settings.HostInterfaceName, err)
	}

	if err := runCommand(
		"ip",
		"link",
		"set",
		peerName,
		"netns",
		strconv.Itoa(containerPID),
		"name",
		settings.ContainerInterfaceName,
	); err != nil {
		cleanup()
		return nil, fmt.Errorf("move %s into container network namespace: %w", peerName, err)
	}

	return cleanup, nil
}

func ensureBridge(name, cidr string) error {
	if err := runCommand("ip", "link", "show", "dev", name); err != nil {
		if err := runCommand("ip", "link", "add", name, "type", "bridge"); err != nil {
			return fmt.Errorf("create bridge %s: %w", name, err)
		}
	}

	if err := runCommand("ip", "addr", "replace", cidr, "dev", name); err != nil {
		return fmt.Errorf("assign %s to bridge %s: %w", cidr, name, err)
	}

	if err := bringInterfaceUp(name); err != nil {
		return fmt.Errorf("bring up bridge %s: %w", name, err)
	}

	return nil
}

func ensureIPv4Forwarding() error {
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0o644); err != nil {
		return fmt.Errorf("enable IPv4 forwarding: %w", err)
	}

	return nil
}

func ensureNATRules(bridgeName, subnetCIDR string) error {
	if err := ensureIptablesRule(
		"nat",
		"POSTROUTING",
		"-s",
		subnetCIDR,
		"!",
		"-o",
		bridgeName,
		"-j",
		"MASQUERADE",
	); err != nil {
		return fmt.Errorf("ensure postrouting masquerade rule: %w", err)
	}

	if err := ensureIptablesRule(
		"filter",
		"FORWARD",
		"-i",
		bridgeName,
		"-j",
		"ACCEPT",
	); err != nil {
		return fmt.Errorf("ensure forward rule from %s: %w", bridgeName, err)
	}

	if err := ensureIptablesRule(
		"filter",
		"FORWARD",
		"-o",
		bridgeName,
		"-m",
		"conntrack",
		"--ctstate",
		"RELATED,ESTABLISHED",
		"-j",
		"ACCEPT",
	); err != nil {
		return fmt.Errorf("ensure forward return rule to %s: %w", bridgeName, err)
	}

	return nil
}

func ensureIptablesRule(table, chain string, rule ...string) error {
	checkArgs := append([]string{"-t", table, "-C", chain}, rule...)
	if err := runCommand("iptables", checkArgs...); err == nil {
		return nil
	}

	appendArgs := append([]string{"-t", table, "-A", chain}, rule...)
	if err := runCommand("iptables", appendArgs...); err != nil {
		return err
	}

	return nil
}

func waitForInterface(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := net.InterfaceByName(name); err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("interface %q did not appear within %s", name, timeout)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}

		return fmt.Errorf("%w: %s", err, message)
	}

	return nil
}
