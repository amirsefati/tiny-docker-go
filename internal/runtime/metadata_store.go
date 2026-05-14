package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultContainersRoot = "/var/lib/tiny-docker/containers"

type MetadataStore struct {
	root string
}

func NewMetadataStore(root string) *MetadataStore {
	return &MetadataStore{root: root}
}

func (s *MetadataStore) NewContainer(request RunRequest) (ContainerConfig, error) {
	id, err := generateContainerID()
	if err != nil {
		return ContainerConfig{}, fmt.Errorf("generate container id: %w", err)
	}

	return ContainerConfig{
		ID:        id,
		Command:   strings.Join(append([]string{request.Command}, request.Args...), " "),
		Hostname:  request.Hostname,
		RootFS:    request.RootFS,
		Status:    StatusRunning,
		CreatedAt: time.Now().UTC(),
		PID:       0,
	}, nil
}

func (s *MetadataStore) Save(config ContainerConfig) error {
	containerDir := filepath.Join(s.root, config.ID)
	if err := os.MkdirAll(containerDir, 0o755); err != nil {
		return fmt.Errorf("create container directory: %w", err)
	}

	configPath := filepath.Join(containerDir, "config.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal container config: %w", err)
	}

	tempPath := configPath + ".tmp"
	if err := os.WriteFile(tempPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write temporary container config: %w", err)
	}

	if err := os.Rename(tempPath, configPath); err != nil {
		return fmt.Errorf("persist container config: %w", err)
	}

	return nil
}

func (s *MetadataStore) List() ([]ContainerConfig, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return []ContainerConfig{}, nil
		}

		return nil, fmt.Errorf("read containers directory: %w", err)
	}

	configs := make([]ContainerConfig, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		config, err := s.load(entry.Name())
		if err != nil {
			return nil, err
		}

		configs = append(configs, config)
	}

	sort.Slice(configs, func(i, j int) bool {
		return configs[i].CreatedAt.After(configs[j].CreatedAt)
	})

	return configs, nil
}

func (s *MetadataStore) load(id string) (ContainerConfig, error) {
	configPath := filepath.Join(s.root, id, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ContainerConfig{}, fmt.Errorf("read container config %q: %w", id, err)
	}

	var config ContainerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ContainerConfig{}, fmt.Errorf("decode container config %q: %w", id, err)
	}

	return config, nil
}

func generateContainerID() (string, error) {
	randomBytes := make([]byte, 6)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(randomBytes), nil
}
