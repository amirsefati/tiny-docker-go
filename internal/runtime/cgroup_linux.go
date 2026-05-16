//go:build linux

package runtime

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const cgroupV2Root = "/sys/fs/cgroup"
const cgroupParentName = "tiny-docker"

type cgroupManager struct {
	path string
}

func newCgroupManager(containerID string) (*cgroupManager, error) {
	if _, err := os.Stat(filepath.Join(cgroupV2Root, "cgroup.controllers")); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("cgroup v2 is not available on this host")
		}

		return nil, fmt.Errorf("detect cgroup v2 support: %w", err)
	}

	parentPath := filepath.Join(cgroupV2Root, cgroupParentName)
	if err := os.MkdirAll(parentPath, 0o755); err != nil {
		return nil, fmt.Errorf("create cgroup parent %q: %w", parentPath, err)
	}

	cgroupPath := filepath.Join(parentPath, containerID)
	if err := os.Mkdir(cgroupPath, 0o755); err != nil {
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create container cgroup %q: %w", cgroupPath, err)
		}
	}

	return &cgroupManager{
		path: cgroupPath,
	}, nil
}

func (m *cgroupManager) ApplyMemoryLimit(limit string) error {
	if strings.TrimSpace(limit) == "" {
		return nil
	}

	limitBytes, err := parseMemoryLimit(limit)
	if err != nil {
		return err
	}

	memoryMaxPath := filepath.Join(m.path, "memory.max")
	if err := os.WriteFile(memoryMaxPath, []byte(strconv.FormatInt(limitBytes, 10)), 0o644); err != nil {
		return fmt.Errorf("set memory limit on %q: %w", memoryMaxPath, err)
	}

	return nil
}

func (m *cgroupManager) AddProcess(pid int) error {
	cgroupProcsPath := filepath.Join(m.path, "cgroup.procs")
	if err := os.WriteFile(cgroupProcsPath, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("add pid %d to cgroup %q: %w", pid, cgroupProcsPath, err)
	}

	return nil
}

func (m *cgroupManager) Cleanup() error {
	var lastErr error

	for range 20 {
		err := os.Remove(m.path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}

		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("remove cgroup %q: %w", m.path, lastErr)
}

func parseMemoryLimit(limit string) (int64, error) {
	trimmed := strings.TrimSpace(strings.ToLower(limit))
	if trimmed == "" {
		return 0, errors.New("memory limit cannot be empty")
	}

	multipliers := map[string]int64{
		"b":  1,
		"k":  1024,
		"kb": 1024,
		"m":  1024 * 1024,
		"mb": 1024 * 1024,
		"g":  1024 * 1024 * 1024,
		"gb": 1024 * 1024 * 1024,
	}

	numberEnd := 0
	for numberEnd < len(trimmed) && trimmed[numberEnd] >= '0' && trimmed[numberEnd] <= '9' {
		numberEnd++
	}

	if numberEnd == 0 {
		return 0, fmt.Errorf("invalid memory limit %q", limit)
	}

	value, err := strconv.ParseInt(trimmed[:numberEnd], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse memory limit %q: %w", limit, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("memory limit must be greater than zero: %q", limit)
	}

	suffix := trimmed[numberEnd:]
	if suffix == "" {
		return value, nil
	}

	multiplier, ok := multipliers[suffix]
	if !ok {
		return 0, fmt.Errorf("unsupported memory suffix in %q", limit)
	}
	if value > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("memory limit is too large: %q", limit)
	}

	return value * multiplier, nil
}
