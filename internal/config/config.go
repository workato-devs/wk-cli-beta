package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// ProjectFile is the name of the project configuration file.
const ProjectFile = "wk.toml"

// Config represents the contents of a wk.toml project file.
//
// Workspace, WorkspaceID, Environment, and Email are an informational
// snapshot of the bound profile at init time (see ADR-006 Sub-decision 8).
// Runtime routing always resolves from the profile store; these fields
// exist so `cat wk.toml` reveals what the project targets at a glance.
type Config struct {
	Name        string      `toml:"name"`
	Description string      `toml:"description,omitempty"`
	Profile     string      `toml:"profile"`
	Workspace   string      `toml:"workspace,omitempty"`
	WorkspaceID int         `toml:"workspace_id,omitempty"`
	Environment string      `toml:"environment,omitempty"`
	Email       string      `toml:"email,omitempty"`
	Plugins     []string    `toml:"plugins,omitempty"`
	MCP         MCPConfig   `toml:"mcp,omitempty"`
	Sync        []SyncEntry `toml:"sync,omitempty"`
}

// MCPConfig holds MCP integration settings.
type MCPConfig struct {
	AutoDelegate bool   `toml:"auto_delegate"`
	ServerURL    string `toml:"server_url,omitempty"`
}

// SyncEntry maps a Workato server-side path to a local directory.
type SyncEntry struct {
	ServerPath string   `toml:"server_path"`
	LocalPath  string   `toml:"local_path"`
	Include    []string `toml:"include,omitempty"`
}

// Load reads and parses a wk.toml file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save writes a Config to a wk.toml file at the given path.
func Save(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// FindProjectRoot walks up from startDir looking for a wk.toml file.
// Returns the directory containing wk.toml, or an error if none is found.
func FindProjectRoot(startDir string) (string, error) {
	dir := startDir
	for {
		configPath := filepath.Join(dir, ProjectFile)
		if _, err := os.Stat(configPath); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found in %s or any parent directory", ProjectFile, startDir)
		}
		dir = parent
	}
}
