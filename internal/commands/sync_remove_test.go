package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

func TestSyncRemove(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack"},
		{ServerPath: "Recipes/GitHub", LocalPath: "./github"},
	})

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "remove", "Recipes/Slack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("remove: %v", err)
	}

	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if len(reloaded.Sync) != 1 || reloaded.Sync[0].ServerPath != "Recipes/GitHub" {
		t.Errorf("Sync = %+v, want only {Recipes/GitHub}", reloaded.Sync)
	}
}

func TestSyncRemove_PurgeDeletesLocalAndMetas(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack"},
	})
	// Drop an asset + mirror meta to confirm --purge nukes both trees.
	assetPath := filepath.Join(cwd, "slack", "r.json")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0755); err != nil {
		t.Fatalf("mkdir asset: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("x"), 0644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	metaPath := filepath.Join(cwd, config.ProjectDir, "slack", "r.json.meta.json")
	if err := os.MkdirAll(filepath.Dir(metaPath), 0755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "remove", "Recipes/Slack", "--purge"})
	if err := root.Execute(); err != nil {
		t.Fatalf("remove --purge: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cwd, "slack")); !os.IsNotExist(err) {
		t.Errorf("local dir should be purged, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, config.ProjectDir, "slack")); !os.IsNotExist(err) {
		t.Errorf("meta dir should be purged, stat err = %v", err)
	}
}

func TestSyncRemove_PurgeRefusesDotPath(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Root", LocalPath: "."},
	})

	// Sentinel file at project root must survive --purge.
	sentinel := filepath.Join(cwd, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "remove", "Root", "--purge"})
	if err := root.Execute(); err != nil {
		t.Fatalf("remove --purge: %v", err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel file should survive --purge on root path: %v", err)
	}
	// Entry should still be removed from config, only the purge is refused.
	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if len(reloaded.Sync) != 0 {
		t.Errorf("sync entry should still be removed from config, got %+v", reloaded.Sync)
	}
}

func TestSyncRemove_NoMatchErrors(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack"},
	})

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "remove", "Recipes/Ghost"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "no sync entry") {
		t.Errorf("err = %v, want 'no sync entry'", err)
	}
}
