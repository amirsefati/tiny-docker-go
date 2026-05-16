//go:build linux

package runtime

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	NetworkModeIsolated = "isolated"
	NetworkModeNone     = "none"
)

type networkConfig struct {
	mode string
}

type networkConfigurator interface {
	Setup() error
}

type isolatedNetworkConfigurator struct{}

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

func newNetworkConfig(mode string) (networkConfig, error) {
	normalized, err := normalizeNetworkMode(mode)
	if err != nil {
		return networkConfig{}, err
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
	case NetworkModeIsolated, NetworkModeNone:
		return isolatedNetworkConfigurator{}
	default:
		return nil
	}
}

func (c networkConfig) Mode() string {
	return c.mode
}

func (isolatedNetworkConfigurator) Setup() error {
	if err := bringInterfaceUp("lo"); err != nil {
		return fmt.Errorf("bring up loopback interface: %w", err)
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
