package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

func TestRegistryInstallListRemove(t *testing.T) {
	// Create temp plugin source with manifest and dummy entrypoint
	srcDir := t.TempDir()
	manifest := `name = "test-plugin"
version = "1.0.0"
description = "Test"
entrypoint = "./test-plugin"

[[commands]]
name = "greet"
description = "Greet"
method = "test.greet"
`
	os.WriteFile(filepath.Join(srcDir, "plugin.toml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(srcDir, "test-plugin"), []byte("#!/bin/sh\necho ok"), 0755)

	// Create temp registry dir
	regDir := t.TempDir()
	reg := &Registry{Dir: regDir}

	// Install
	if err := reg.Install(srcDir); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// List
	plugins, err := reg.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", plugins[0].Name)
	}

	// Remove
	if err := reg.Remove("test-plugin"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	plugins, _ = reg.List()
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins after remove, got %d", len(plugins))
	}
}

func TestRegistryRemoveNotInstalled(t *testing.T) {
	reg := &Registry{Dir: t.TempDir()}
	err := reg.Remove("nonexistent")
	if err == nil {
		t.Fatal("expected error removing non-existent plugin")
	}
	if !errors.Is(err, wkerrors.ErrPluginNotFound) {
		t.Errorf("expected errors.Is(err, ErrPluginNotFound), got: %v", err)
	}
	want := `plugin "nonexistent" is not installed`
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestRegistryGetPluginDirNotInstalled(t *testing.T) {
	reg := &Registry{Dir: t.TempDir()}
	_, err := reg.GetPluginDir("missing")
	if err == nil {
		t.Fatal("expected error for non-existent plugin")
	}
	if !errors.Is(err, wkerrors.ErrPluginNotFound) {
		t.Errorf("expected errors.Is(err, ErrPluginNotFound), got: %v", err)
	}
	want := `plugin "missing" is not installed`
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestRegistryListEmpty(t *testing.T) {
	reg := &Registry{Dir: t.TempDir()}
	plugins, err := reg.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0, got %d", len(plugins))
	}
}
