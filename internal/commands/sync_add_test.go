package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// writeProjectSkel creates a minimal wk project at cwd with the given sync
// entries so sync-entry commands can operate against it. Shared with other
// sync_*_test.go files in this package.
func writeProjectSkel(t *testing.T, cwd string, entries []config.SyncEntry) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(cwd, config.ProjectDir), 0755); err != nil {
		t.Fatalf("mkdir .wk: %v", err)
	}
	cfg := &config.Config{Name: "p", Profile: "dev", Sync: entries}
	if err := config.Save(config.ProjectConfigPath(cwd), cfg); err != nil {
		t.Fatalf("save cfg: %v", err)
	}
}

func TestSyncAdd_WithSyncFlag(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, nil)

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "add", "--sync", "Recipes/Slack:./slack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	reloaded, err := config.Load(config.ProjectConfigPath(cwd))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(reloaded.Sync) != 1 || reloaded.Sync[0].ServerPath != "Recipes/Slack" || reloaded.Sync[0].LocalPath != "./slack" {
		t.Errorf("Sync = %+v, want one {Recipes/Slack, ./slack}", reloaded.Sync)
	}
}

func TestSyncAdd_WithProjectFlag(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, nil)

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "add", "--project", "alpha", "--project", "beta"})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if len(reloaded.Sync) != 2 {
		t.Fatalf("Sync len = %d, want 2 (%+v)", len(reloaded.Sync), reloaded.Sync)
	}
	want := map[string]string{"alpha": "alpha", "beta": "beta"}
	for _, e := range reloaded.Sync {
		if w, ok := want[e.ServerPath]; !ok || e.LocalPath != w {
			t.Errorf("entry %+v does not match any expected (want %+v)", e, want)
		}
	}
}

func TestSyncAdd_DefaultLocalPathFromSync(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, nil)

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "add", "--sync", "Recipes/Slack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if reloaded.Sync[0].LocalPath != "./Slack" {
		t.Errorf("LocalPath = %q, want ./Slack (derived)", reloaded.Sync[0].LocalPath)
	}
}

// TestSyncAdd_SilentlySkipsExistingExactMatch pins ADR-007 Decision 8 point 7:
// an entry that exactly matches an existing wk.toml entry is a valid no-op
// (silent skip), not a hard error. Contrast with same-invocation duplicates,
// which still error per PR B's typo-catcher semantics.
func TestSyncAdd_SilentlySkipsExistingExactMatch(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./Slack"},
	})

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "add", "--sync", "Recipes/Slack:./Slack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v (want silent no-op, not error)", err)
	}

	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if len(reloaded.Sync) != 1 {
		t.Errorf("Sync len = %d, want 1 (entry should not be duplicated): %+v",
			len(reloaded.Sync), reloaded.Sync)
	}
}

func TestSyncAdd_ErrorsOnSameInvocationDuplicate(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, nil)

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "add", "--project", "alpha", "--project", "alpha"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Errorf("err = %v, want 'declared twice'", err)
	}
}

func TestSyncAdd_ErrorsWhenNoEntriesDeclared(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, nil)

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "add"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "no entries to add") {
		t.Errorf("err = %v, want 'no entries to add'", err)
	}
}

// TestSyncAdd_AcceptsNoInput regression-guards the bug where `wk sync add`
// rejected --no-input while `wk init` accepted it. --no-input is a root
// persistent flag; commands that don't prompt accept it as a no-op so
// scripts can pass it uniformly.
func TestSyncAdd_AcceptsNoInput(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, nil)

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "add", "--no-input", "--project", "alpha"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync add --no-input rejected: %v", err)
	}

	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if len(reloaded.Sync) != 1 {
		t.Errorf("Sync len = %d, want 1 (%+v)", len(reloaded.Sync), reloaded.Sync)
	}
}
