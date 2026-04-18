package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

func TestParseSyncFlag(t *testing.T) {
	tests := []struct {
		in         string
		wantServer string
		wantLocal  string
		wantErr    bool
	}{
		{"Recipes/Slack", "Recipes/Slack", "./Slack", false},
		{"Recipes/Slack:./code/slack", "Recipes/Slack", "./code/slack", false},
		{"Recipes/Slack:", "Recipes/Slack", "./Slack", false},
		{"All projects/Recipes:./rx", "All projects/Recipes", "./rx", false},
		{"  Recipes/Slack  ", "Recipes/Slack", "./Slack", false},
		{":/missing-server", "", "", true},
		{"", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			e, err := parseSyncFlag(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSyncFlag(%q) err = nil, want err", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSyncFlag(%q) err = %v", tc.in, err)
			}
			if e.ServerPath != tc.wantServer || e.LocalPath != tc.wantLocal {
				t.Errorf("parseSyncFlag(%q) = {%q, %q}, want {%q, %q}",
					tc.in, e.ServerPath, e.LocalPath, tc.wantServer, tc.wantLocal)
			}
		})
	}
}

func TestDefaultLocalPathForServerPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Recipes/Slack", "./Slack"},
		{"Recipes", "./Recipes"},
		{"All projects/Recipes/Slack", "./Slack"},
		{"All projects", "."},
		{"/Recipes/Slack/", "./Slack"},
		{"", "."},
	}
	for _, tc := range tests {
		if got := defaultLocalPathForServerPath(tc.in); got != tc.want {
			t.Errorf("defaultLocalPathForServerPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// writeProjectSkel creates a minimal wk project at cwd with the given sync
// entries so project-sync commands can operate.
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

func TestProjectSyncAdd(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, nil)

	root := NewRootCmd()
	root.AddCommand(newProjectCmd())
	root.SetArgs([]string{"project", "sync", "add", "Recipes/Slack", "./slack"})
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

func TestProjectSyncAdd_DuplicateRejected(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack"},
	})

	root := NewRootCmd()
	root.AddCommand(newProjectCmd())
	root.SetArgs([]string{"project", "sync", "add", "Recipes/Slack", "./slack"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want 'already exists'", err)
	}
}

func TestProjectSyncAdd_DefaultLocalPath(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, nil)

	root := NewRootCmd()
	root.AddCommand(newProjectCmd())
	root.SetArgs([]string{"project", "sync", "add", "Recipes/Slack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if reloaded.Sync[0].LocalPath != "./Slack" {
		t.Errorf("LocalPath = %q, want ./Slack (derived)", reloaded.Sync[0].LocalPath)
	}
}

func TestProjectSyncRemove(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack"},
		{ServerPath: "Recipes/GitHub", LocalPath: "./github"},
	})

	root := NewRootCmd()
	root.AddCommand(newProjectCmd())
	root.SetArgs([]string{"project", "sync", "remove", "Recipes/Slack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("remove: %v", err)
	}

	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if len(reloaded.Sync) != 1 || reloaded.Sync[0].ServerPath != "Recipes/GitHub" {
		t.Errorf("Sync = %+v, want only {Recipes/GitHub}", reloaded.Sync)
	}
}

func TestProjectSyncRemove_PurgeDeletesLocalAndMetas(t *testing.T) {
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
	root.AddCommand(newProjectCmd())
	root.SetArgs([]string{"project", "sync", "remove", "Recipes/Slack", "--purge"})
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

func TestProjectSyncRemove_PurgeRefusesDotPath(t *testing.T) {
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
	root.AddCommand(newProjectCmd())
	root.SetArgs([]string{"project", "sync", "remove", "Root", "--purge"})
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

func TestProjectSyncRemove_NoMatchErrors(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack"},
	})

	root := NewRootCmd()
	root.AddCommand(newProjectCmd())
	root.SetArgs([]string{"project", "sync", "remove", "Recipes/Ghost"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "no sync entry") {
		t.Errorf("err = %v, want 'no sync entry'", err)
	}
}

func TestProjectSyncList(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack", FolderID: 42},
		{ServerPath: "Recipes/GitHub", LocalPath: "./github"},
	})

	var out bytes.Buffer
	root := NewRootCmd()
	root.AddCommand(newProjectCmd())
	root.SetOut(&out)
	root.SetArgs([]string{"project", "sync", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
	// formatter writes to os.Stdout, not the cobra out — sanity-check at least
	// that no error occurred and the config still holds both entries.
	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if len(reloaded.Sync) != 2 {
		t.Errorf("list should not mutate config: got %+v", reloaded.Sync)
	}
}

func TestInitWithRepeatedSyncFlag(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()
	resetGlobalFlags(t)

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{
		"init",
		"--name", "multi-proj",
		"--profile", "dev",
		"--sync", "Recipes/Slack:./slack",
		"--sync", "Recipes/GitHub",
		"--json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(filepath.Join(dir, "multi-proj")))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Sync) != 2 {
		t.Fatalf("Sync len = %d, want 2 (%+v)", len(cfg.Sync), cfg.Sync)
	}
	if cfg.Sync[0].ServerPath != "Recipes/Slack" || cfg.Sync[0].LocalPath != "./slack" {
		t.Errorf("entry 0 = %+v, want {Recipes/Slack, ./slack}", cfg.Sync[0])
	}
	if cfg.Sync[1].ServerPath != "Recipes/GitHub" || cfg.Sync[1].LocalPath != "./GitHub" {
		t.Errorf("entry 1 = %+v, want {Recipes/GitHub, ./GitHub}", cfg.Sync[1])
	}
}

func TestInitRejectsLocalPathWithoutServerPath(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()
	resetGlobalFlags(t)

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{
		"init",
		"--name", "solo",
		"--profile", "dev",
		"--local-path", "./lonely",
		"--sync", "Recipes/Slack",
		"--json",
	})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--local-path requires --server-path") {
		t.Errorf("err = %v, want '--local-path requires --server-path'", err)
	}
}

func TestInitCombinesShorthandAndSyncFlag(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()
	resetGlobalFlags(t)

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{
		"init",
		"--name", "mix-proj",
		"--profile", "dev",
		"--server-path", "Recipes/Primary",
		"--local-path", "./primary",
		"--sync", "Recipes/Secondary:./secondary",
		"--json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	cfg, _ := config.Load(config.ProjectConfigPath(filepath.Join(dir, "mix-proj")))
	if len(cfg.Sync) != 2 {
		t.Fatalf("Sync len = %d, want 2 (%+v)", len(cfg.Sync), cfg.Sync)
	}
}
