package plugin

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Registry manages installed plugins on disk at ~/.wk/plugins/.
type Registry struct {
	Dir string
}

// InstalledPlugin describes a plugin found in the registry.
type InstalledPlugin struct {
	Name    string
	Version string
	Dir     string
}

// NewRegistry creates a Registry with the default directory (~/.wk/plugins/).
// It creates the directory if it does not exist.
// If WK_HOME is set, it is used instead of ~/.wk/ (useful for testing).
func NewRegistry() (*Registry, error) {
	base := os.Getenv("WK_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("determining home directory: %w", err)
		}
		base = filepath.Join(home, ".wk")
	}
	dir := filepath.Join(base, "plugins")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating plugins directory: %w", err)
	}
	return &Registry{Dir: dir}, nil
}

// Install copies a plugin from source into the registry.
// It reads the plugin.toml at source to determine the plugin name.
func (r *Registry) Install(source string) error {
	manifestPath := filepath.Join(source, "plugin.toml")
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("reading plugin manifest: %w", err)
	}

	dest := filepath.Join(r.Dir, m.Name)

	// Remove existing installation if present
	if _, err := os.Stat(dest); err == nil {
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("removing existing plugin: %w", err)
		}
	}

	if err := copyDir(source, dest); err != nil {
		return fmt.Errorf("installing plugin: %w", err)
	}
	return nil
}

// List scans the registry directory for installed plugins.
func (r *Registry) List() ([]InstalledPlugin, error) {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return nil, fmt.Errorf("reading plugins directory: %w", err)
	}

	var plugins []InstalledPlugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(r.Dir, entry.Name(), "plugin.toml")
		m, err := LoadManifest(manifestPath)
		if err != nil {
			continue // skip directories without valid manifests
		}
		plugins = append(plugins, InstalledPlugin{
			Name:    m.Name,
			Version: m.Version,
			Dir:     filepath.Join(r.Dir, entry.Name()),
		})
	}
	return plugins, nil
}

// Remove deletes an installed plugin by name.
func (r *Registry) Remove(name string) error {
	dir := filepath.Join(r.Dir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("plugin %q is not installed", name)
	}
	return os.RemoveAll(dir)
}

// GetPluginDir returns the path to an installed plugin's directory.
func (r *Registry) GetPluginDir(name string) (string, error) {
	dir := filepath.Join(r.Dir, name)
	if _, err := os.Stat(filepath.Join(dir, "plugin.toml")); err != nil {
		return "", fmt.Errorf("plugin %q is not installed", name)
	}
	return dir, nil
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
