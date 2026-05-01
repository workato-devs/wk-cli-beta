package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// setupTestHome creates a temporary HOME directory with a .wk/profiles.json
// containing a "dev" profile, and sets it as the active profile. Returns a
// cleanup function that restores the original HOME.
func setupTestHome(t *testing.T) func() {
	t.Helper()
	origHome := os.Getenv("HOME")

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	wkDir := filepath.Join(tmpHome, ".wk")
	os.MkdirAll(wkDir, 0700)

	profiles := []*auth.Profile{
		{
			Name:        "dev",
			Workspace:   "test-workspace",
			WorkspaceID: 12345,
			Environment: "dev",
			Email:       "dev@example.com",
			Region:      auth.RegionUS,
			StoreType:   auth.StoreKeychain,
			BaseURL:     "https://www.workato.com",
		},
	}
	data, _ := json.Marshal(profiles)
	os.WriteFile(filepath.Join(wkDir, "profiles.json"), data, 0600)
	os.WriteFile(filepath.Join(wkDir, "active_profile"), []byte("dev"), 0600)

	return func() {
		os.Setenv("HOME", origHome)
	}
}

func TestInitSyncFlagDefaults(t *testing.T) {
	tests := []struct {
		name      string
		sync      string
		wantLocal string
	}{
		{
			name:      "bare name defaults to ./<name>",
			sync:      "Test",
			wantLocal: "./Test",
		},
		{
			name:      "nested server path uses last segment",
			sync:      "Dev Team Testing/Gong.io API",
			wantLocal: "./Gong.io API",
		},
		{
			name:      "explicit local path preserved",
			sync:      "Test:./custom",
			wantLocal: "./custom",
		},
		{
			name:      "trailing slash uses last segment",
			sync:      "Test/Sub/",
			wantLocal: "./Sub",
		},
		{
			name:      "All projects prefix stripped",
			sync:      "All projects/Team/A",
			wantLocal: "./A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanupHome := setupTestHome(t)
			defer cleanupHome()

			dir := t.TempDir()
			origDir, _ := os.Getwd()
			os.Chdir(dir)
			defer os.Chdir(origDir)

			root := NewRootCmd()
			root.AddCommand(newInitCmd())
			root.SetArgs([]string{
				"init",
				"--name", "test-proj",
				"--profile", "dev",
				"--sync", tt.sync,
				"--json",
			})
			if err := root.Execute(); err != nil {
				t.Fatalf("init failed: %v", err)
			}

			cfg, err := config.Load(config.ProjectConfigPath(filepath.Join(dir, "test-proj")))
			if err != nil {
				t.Fatalf("loading config: %v", err)
			}
			if len(cfg.Sync) != 1 {
				t.Fatalf("Sync len = %d, want 1", len(cfg.Sync))
			}
			if cfg.Sync[0].LocalPath != tt.wantLocal {
				t.Errorf("LocalPath = %q, want %q", cfg.Sync[0].LocalPath, tt.wantLocal)
			}
		})
	}
}

func TestInitCreatesContainerDirectory(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "my-project", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Container directory should exist.
	projectDir := filepath.Join(dir, "my-project")
	info, err := os.Stat(projectDir)
	if err != nil {
		t.Fatalf("project directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory, got file")
	}

	// wk.toml should be inside the container.
	cfg, err := config.Load(config.ProjectConfigPath(projectDir))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Name != "my-project" {
		t.Errorf("Name = %q, want %q", cfg.Name, "my-project")
	}
	if cfg.Profile != "dev" {
		t.Errorf("Profile = %q, want %q", cfg.Profile, "dev")
	}

	// wk.toml should NOT exist in the parent (CWD).
	if _, err := os.Stat(filepath.Join(dir, config.ProjectFile)); err == nil {
		t.Error("wk.toml should not exist in CWD, only inside the container directory")
	}
}

func TestInitScaffoldsIntoExistingDir(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Pre-create the directory.
	projectDir := filepath.Join(dir, "existing-project")
	if err := os.Mkdir(projectDir, 0755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "existing-project", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init into existing dir failed: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(projectDir))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Name != "existing-project" {
		t.Errorf("Name = %q, want %q", cfg.Name, "existing-project")
	}
}

func TestInitErrorsOnExistingProject(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Create a project directory with .wk/wk.toml already inside.
	projectDir := filepath.Join(dir, "existing")
	os.MkdirAll(filepath.Join(projectDir, config.ProjectDir), 0755)
	config.Save(config.ProjectConfigPath(projectDir), &config.Config{Name: "existing"})

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "existing", "--profile", "dev", "--json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for existing project, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want it to mention 'already exists'", err.Error())
	}
}

func TestInitNestingGuard(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Create .wk/wk.toml in the temp dir to simulate being inside a project.
	os.MkdirAll(filepath.Join(dir, config.ProjectDir), 0755)
	config.Save(config.ProjectConfigPath(dir), &config.Config{Name: "parent"})

	// cd into a subdirectory of the project.
	subDir := filepath.Join(dir, "subdir")
	os.Mkdir(subDir, 0755)
	os.Chdir(subDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "nested", "--profile", "dev", "--json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected nesting guard error, got nil")
	}
	if !strings.Contains(err.Error(), "inside an existing wk project") {
		t.Errorf("error = %q, want nesting guard message", err.Error())
	}
}

func TestInitProfileValidation(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Use a profile that doesn't exist.
	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "my-project", "--profile", "nonexistent", "--json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected profile not found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want profile not found message", err.Error())
	}
}

func TestInitWritesProfileSnapshot(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "snap-project", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(filepath.Join(dir, "snap-project")))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Workspace != "test-workspace" {
		t.Errorf("Workspace = %q, want %q", cfg.Workspace, "test-workspace")
	}
	if cfg.WorkspaceID != 12345 {
		t.Errorf("WorkspaceID = %d, want 12345", cfg.WorkspaceID)
	}
	if cfg.Environment != "dev" {
		t.Errorf("Environment = %q, want %q", cfg.Environment, "dev")
	}
	if cfg.Email != "dev@example.com" {
		t.Errorf("Email = %q, want %q", cfg.Email, "dev@example.com")
	}
}

func TestInit_NonInteractiveFailsFast(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantMissing []string
	}{
		{"json no flags", []string{"init", "--json"}, []string{"--name", "--profile"}},
		{"json name only", []string{"init", "--json", "--name", "x"}, []string{"--profile"}},
		{"no-input no flags", []string{"init", "--no-input"}, []string{"--name", "--profile"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanupHome := setupTestHome(t)
			defer cleanupHome()

			dir := t.TempDir()
			origDir, _ := os.Getwd()
			os.Chdir(dir)
			defer os.Chdir(origDir)

			root := NewRootCmd()
			root.AddCommand(newInitCmd())
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil {
				t.Fatal("expected non-interactive validation error, got nil")
			}
			msg := err.Error()
			for _, flag := range tc.wantMissing {
				if !strings.Contains(msg, flag) {
					t.Errorf("err = %q, want substring %q", msg, flag)
				}
			}
			if !strings.Contains(msg, "non-interactive mode") {
				t.Errorf("err = %q, want mention of non-interactive mode", msg)
			}
			if strings.Contains(msg, "Project name:") || strings.Contains(msg, "Auth profile:") {
				t.Errorf("err contains a prompt label: %q", msg)
			}
		})
	}
}

func TestInitStoreTypeFile_WarnsWhenMissing(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Capture stderr.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "file-proj", "--profile", "ci", "--store-type", "file", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	w.Close()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	stderr := string(buf[:n])
	if !strings.Contains(stderr, "--store-type file specified but no profiles.env") {
		t.Errorf("expected warn about missing profiles.env, got %q", stderr)
	}

	// wk.toml should exist with profile reference but no snapshot fields.
	cfg, err := config.Load(config.ProjectConfigPath(filepath.Join(dir, "file-proj")))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Profile != "ci" {
		t.Errorf("Profile = %q, want ci", cfg.Profile)
	}
	if cfg.Workspace != "" {
		t.Errorf("Workspace = %q, want empty (deferred)", cfg.Workspace)
	}
}

func TestInitStoreTypeFile_HydratesFromProfilesEnv(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Pre-create the target directory and drop profiles.env at its root.
	// profiles.env lives at <projectRoot>/profiles.env per ADR-006
	// Sub-decision 3 (April 20, 2026 revision) — project root, outside
	// .wk/, so the file can exist before init creates .wk/. init will
	// append `profiles.env` to the project's .gitignore as the safety net
	// that the .wk/ self-gitignore used to provide.
	projectDir := filepath.Join(dir, "ci-proj")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	profilesEnv := "NAME=ci\nWORKSPACE=acme\nENVIRONMENT=prod\nREGION=us\nTOKEN=secret\n"
	if err := os.WriteFile(auth.NewFileStore(projectDir).Path, []byte(profilesEnv), 0600); err != nil {
		t.Fatalf("writing profiles.env: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "ci-proj", "--profile", "ci", "--store-type", "file", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(projectDir))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Workspace != "acme" {
		t.Errorf("Workspace = %q, want acme", cfg.Workspace)
	}
	if cfg.Environment != "prod" {
		t.Errorf("Environment = %q, want prod", cfg.Environment)
	}
}

func TestInitWritesWkGitignore(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "gitigproj", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// .wk/.gitignore should exist with the self-ignore content.
	wkIgnore := filepath.Join(dir, "gitigproj", config.ProjectDir, ".gitignore")
	data, err := os.ReadFile(wkIgnore)
	if err != nil {
		t.Fatalf("reading %s: %v", wkIgnore, err)
	}
	body := string(data)
	if !strings.Contains(body, "\n*\n") {
		t.Errorf(".wk/.gitignore missing \"*\" pattern, got:\n%s", body)
	}
	if !strings.Contains(body, "!.gitignore") {
		t.Errorf(".wk/.gitignore missing \"!.gitignore\" re-inclusion, got:\n%s", body)
	}

	// Project-root .gitignore MUST exist with a profiles.env entry — the
	// safety net replacing the .wk/ self-gitignore coverage that was lost
	// when profiles.env moved to project root (ADR-006 Sub-decision 3,
	// April 20 revision).
	rootIgnore := filepath.Join(dir, "gitigproj", ".gitignore")
	rootBody, err := os.ReadFile(rootIgnore)
	if err != nil {
		t.Fatalf("reading project-root .gitignore: %v", err)
	}
	hasEntry := false
	for _, line := range strings.Split(string(rootBody), "\n") {
		if strings.TrimSpace(line) == "profiles.env" {
			hasEntry = true
			break
		}
	}
	if !hasEntry {
		t.Errorf("project-root .gitignore missing profiles.env entry, got:\n%s", rootBody)
	}
}

func TestInitAppendsProfilesEnvToProjectRootGitignore(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Pre-create target with a developer-owned .gitignore containing
	// unrelated entries. init should append `profiles.env` without
	// disturbing the existing lines.
	projectDir := filepath.Join(dir, "devproj")
	os.MkdirAll(projectDir, 0755)
	preExisting := "node_modules/\ndist/\n"
	rootIgnore := filepath.Join(projectDir, ".gitignore")
	if err := os.WriteFile(rootIgnore, []byte(preExisting), 0644); err != nil {
		t.Fatalf("seeding .gitignore: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "devproj", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Existing lines must be preserved verbatim; `profiles.env` must be
	// appended after them.
	want := preExisting + "profiles.env\n"
	got, err := os.ReadFile(rootIgnore)
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if string(got) != want {
		t.Errorf("project-root .gitignore mismatch.\nwant: %q\ngot:  %q", want, got)
	}
}

func TestInitProjectRootGitignoreIdempotent(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Pre-seed .gitignore with profiles.env already present. init must
	// not duplicate the line.
	projectDir := filepath.Join(dir, "devproj")
	os.MkdirAll(projectDir, 0755)
	preExisting := "profiles.env\nnode_modules/\n"
	rootIgnore := filepath.Join(projectDir, ".gitignore")
	if err := os.WriteFile(rootIgnore, []byte(preExisting), 0644); err != nil {
		t.Fatalf("seeding .gitignore: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "devproj", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	got, err := os.ReadFile(rootIgnore)
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if string(got) != preExisting {
		t.Errorf("init should not rewrite .gitignore when entry already present.\nwant: %q\ngot:  %q", preExisting, got)
	}
}

func TestInitWkGitignoreIdempotent(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// First init.
	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "idemproj", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first init: %v", err)
	}

	wkIgnore := filepath.Join(dir, "idemproj", config.ProjectDir, ".gitignore")
	first, err := os.ReadFile(wkIgnore)
	if err != nil {
		t.Fatalf("reading .wk/.gitignore: %v", err)
	}

	// Re-run init with --overwrite; .wk/.gitignore should be byte-for-byte
	// identical afterwards (CLI writes a fixed body).
	root = NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "idemproj", "--profile", "dev", "--overwrite", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("overwrite init: %v", err)
	}
	second, err := os.ReadFile(wkIgnore)
	if err != nil {
		t.Fatalf("reading .wk/.gitignore after overwrite: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf(".wk/.gitignore drifted after --overwrite.\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestInitOverwriteFlag(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// First init creates the project.
	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "overproj", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Second init without --overwrite should fail in non-interactive mode.
	root = NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "overproj", "--profile", "dev", "--json"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error re-initing without --overwrite, got nil")
	}

	// With --overwrite it should succeed.
	root = NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "overproj", "--profile", "dev", "--overwrite", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init --overwrite failed: %v", err)
	}
}

// TestInitOverwriteReplacesSyncEntries pins the ADR-007 semantic: --overwrite
// means replace. Existing [[sync]] entries are dropped, not preserved. Prior
// behavior (preservation, the Issue #29 workaround) was obsoleted by the
// --project/--projects-dir/--sync flag surface, which lets developers declare
// multi-entry configs directly rather than through hand-editing.
func TestInitOverwriteReplacesSyncEntries(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "replaceproj", "--profile", "dev",
		"--sync", "Team/Old:./old", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	configPath := config.ProjectConfigPath(filepath.Join(dir, "replaceproj"))
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	cfg.Sync = append(cfg.Sync, config.SyncEntry{ServerPath: "Team/HandEdited", LocalPath: "./handedit"})
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("saving edited config: %v", err)
	}

	root = NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "replaceproj", "--profile", "dev", "--overwrite",
		"--sync", "Team/New:./new", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("overwrite init failed: %v", err)
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	if len(reloaded.Sync) != 1 {
		t.Fatalf("sync entries after overwrite = %d, want 1 (only Team/New): %+v",
			len(reloaded.Sync), reloaded.Sync)
	}
	if reloaded.Sync[0].ServerPath != "Team/New" {
		t.Errorf("entry 0 server_path = %q, want Team/New", reloaded.Sync[0].ServerPath)
	}
}

func TestInitRejectsTraversalNames(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	cases := []struct {
		name     string
		flagName string
	}{
		{"parent traversal", "../evil"},
		{"pure parent", ".."},
		{"deeper traversal", "../../../etc"},
		{"embedded traversal", "foo/../bar"},
		{"path separator", "foo/bar"},
		{"backslash separator", "foo\\bar"},
		{"absolute path", "/tmp/evil"},
		{"dot", "."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			origDir, _ := os.Getwd()
			os.Chdir(dir)
			defer os.Chdir(origDir)

			root := NewRootCmd()
			root.AddCommand(newInitCmd())
			root.SetArgs([]string{"init", "--name", tc.flagName, "--profile", "dev", "--json"})
			err := root.Execute()
			if err == nil {
				t.Fatalf("expected error for name %q, got nil", tc.flagName)
			}

			// Verify no files leaked outside the temp dir. We check the
			// parent of the temp dir for any .wk/ that shouldn't exist.
			parent := filepath.Dir(dir)
			if entries, _ := os.ReadDir(parent); entries != nil {
				for _, e := range entries {
					if e.Name() == ".wk" || e.Name() == "evil" || e.Name() == "etc" {
						t.Errorf("traversal leaked file %q into parent %s", e.Name(), parent)
					}
				}
			}
		})
	}
}

func TestInitRejectsWhitespaceInName(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// The ` my-project` form (leading space) is invisible in many shell
	// contexts — surface it loudly instead of silently creating a weirdly
	// named directory.
	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", " my-project", "--profile", "dev", "--json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for leading-whitespace name, got nil")
	}
	if !strings.Contains(err.Error(), "whitespace") {
		t.Errorf("error = %q, want message about whitespace", err.Error())
	}

	// The directory should not have been created.
	if _, err := os.Stat(filepath.Join(dir, " my-project")); !os.IsNotExist(err) {
		t.Errorf("directory with leading whitespace should not exist, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "my-project")); !os.IsNotExist(err) {
		t.Errorf("trimmed directory should not exist either (we reject, not silently trim), got err=%v", err)
	}
}

func TestValidateProjectName(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"my-project", false},
		{"my_project_42", false},
		{"a.b", false}, // dots inside name OK, just not "." or ".."
		{"", true},
		{".", true},
		{"..", true},
		{"../x", true},
		{"foo/bar", true},
		{"foo\\bar", true},
		{"foo\x00bar", true},
		// Leading/trailing whitespace — invisible typos from unquoted shells.
		{" my-project", true},
		{"my-project ", true},
		{"\tmy-project", true},
		{"my-project\n", true},
		{"   ", true}, // whitespace-only collapses to whitespace-rejected
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			err := validateProjectName(tc.in)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateProjectName(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestInitActiveProfileMismatch(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	// Add a second profile "staging" but keep "dev" as active.
	tmpHome := os.Getenv("HOME")
	pm := &auth.ProfileManager{Dir: filepath.Join(tmpHome, ".wk")}
	pm.SaveProfile(&auth.Profile{
		Name:        "staging",
		Workspace:   "test-workspace",
		Environment: "staging",
		Region:      auth.RegionUS,
		StoreType:   auth.StoreKeychain,
		BaseURL:     "https://www.workato.com",
	})

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Active is "dev" but we target "staging" — should fail.
	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "my-project", "--profile", "staging", "--json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected active profile mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("error = %q, want active profile mismatch message", err.Error())
	}
}

// TestInitProjectFlagMajorityCase covers the ADR-007 "happy path":
// `wk init --name X --project foo --project bar` at a clean directory
// scaffolds a container with two top-level Workato projects at the
// container root. Validates the default --projects-dir of "." composes
// correctly with repeatable --project flags.
func TestInitProjectFlagMajorityCase(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{
		"init",
		"--name", "my-project",
		"--profile", "dev",
		"--project", "foo",
		"--project", "bar",
		"--json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(filepath.Join(dir, "my-project")))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Sync) != 2 {
		t.Fatalf("Sync len = %d, want 2 (%+v)", len(cfg.Sync), cfg.Sync)
	}
	want := map[string]string{
		"foo": "foo",
		"bar": "bar",
	}
	for _, e := range cfg.Sync {
		if got, ok := want[e.ServerPath]; !ok {
			t.Errorf("unexpected server_path %q", e.ServerPath)
		} else if e.LocalPath != got {
			t.Errorf("entry %q local_path = %q, want %q", e.ServerPath, e.LocalPath, got)
		}
	}
}

// TestInitProjectsDirDiscovery covers the monorepo-shaped flow from ADR-007:
// `wk init --name X --projects-dir ./workato/recipes` walks the directory
// one level deep and creates one entry per non-hidden subdirectory.
// --projects-dir is interpreted *relative to the container* (Decision 1),
// so the on-disk walk resolves to <container>/<projects-dir>/ and each
// resulting local_path stays container-relative.
func TestInitProjectsDirDiscovery(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Workato asset subdirs need to already exist at <container>/<projects-dir>
	// before init walks. Mirrors the dewy-resort workflow: the developer
	// clones the repo, then runs `wk init` with --projects-dir pointing at
	// the existing recipe tree.
	container := filepath.Join(dir, "monorepo")
	recipeDir := filepath.Join(container, "workato", "recipes")
	for _, sub := range []string{"atomic-salesforce", "atomic-stripe", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(recipeDir, sub), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{
		"init",
		"--name", "monorepo",
		"--profile", "dev",
		"--projects-dir", "./workato/recipes",
		"--json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(container))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Sync) != 2 {
		t.Fatalf("Sync len = %d, want 2 (hidden dir should be skipped): %+v", len(cfg.Sync), cfg.Sync)
	}
	wantLocal := map[string]string{
		"atomic-salesforce": filepath.Join("workato/recipes", "atomic-salesforce"),
		"atomic-stripe":     filepath.Join("workato/recipes", "atomic-stripe"),
	}
	for _, e := range cfg.Sync {
		want, ok := wantLocal[e.ServerPath]
		if !ok {
			t.Errorf("unexpected server_path %q", e.ServerPath)
			continue
		}
		if e.LocalPath != want {
			t.Errorf("entry %q local_path = %q, want %q (container-relative, no container-name prefix)",
				e.ServerPath, e.LocalPath, want)
		}
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

// TestInitCombinesProjectAndSyncFlag verifies the ADR-007 flag surface lets
// --project and --sync coexist in one invocation. Replaces the old
// shorthand+sync combination test; the --server-path/--local-path shorthand
// was removed in ADR-007 Decision 4.
func TestInitCombinesProjectAndSyncFlag(t *testing.T) {
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
		"--project", "primary",
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

// TestInitRejectsConflictingServerPaths pins Decision 5 rule 2: declaring
// the same server_path with different local_paths in a single invocation
// is a hard error.
func TestInitRejectsConflictingServerPaths(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{
		"init",
		"--name", "conflict-proj",
		"--profile", "dev",
		"--project", "foo",
		"--sync", "foo:./somewhere-else",
		"--json",
	})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "conflicting local paths") {
		t.Errorf("err = %v, want 'conflicting local paths'", err)
	}
}

// Issue #40 Symptom 1: `wk init` without --store-type, with --profile P where
// P only exists in <targetDir>/profiles.env (no keychain entry). Before the
// April 20 revision this failed with "profile not found — run 'wk auth login'
// first" because init only consulted profiles.json in the default branch.
func TestInitResolvesFileStoreProfileImplicitly(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()
	// Remove the seeded "dev" profile from profiles.json so init must fall
	// through to the file-store branch of the implicit resolver.
	os.Remove(filepath.Join(os.Getenv("HOME"), ".wk", "profiles.json"))
	os.Remove(filepath.Join(os.Getenv("HOME"), ".wk", "active_profile"))

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	targetDir := filepath.Join(dir, "fs-implicit")
	os.MkdirAll(targetDir, 0755)
	body := "NAME=e2e\nWORKSPACE=acme\nWORKSPACE_ID=42\nENVIRONMENT=dev\nEMAIL=e2e@acme.corp\nREGION=us\nSTORE_TYPE=file\nTOKEN=tok\n"
	if err := os.WriteFile(auth.NewFileStore(targetDir).Path, []byte(body), 0600); err != nil {
		t.Fatalf("seeding profiles.env: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "fs-implicit", "--profile", "e2e", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(targetDir))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Workspace != "acme" {
		t.Errorf("Workspace = %q, want acme (snapshot from file-store profile)", cfg.Workspace)
	}
}

// Issue #40 Symptom 2: `wk init --store-type file --verify` from parent dir
// where the profile lives in <targetDir>/profiles.env. Before the April 20
// revision this failed at the --verify path because resolveVerifyClient
// routed through fileStoreForCwd (CWD walk-up), which can't find a project
// that doesn't exist yet. After the fix, resolveVerifyClientInDir anchors
// on targetDir and the profile resolves even though init runs from outside
// the project.
//
// We don't exercise the real API walk here (no live Workato); the test
// validates that profile+credential resolution succeeds, which is the path
// that was broken. A failing resolver errors BEFORE the API call with the
// "-store-type file requires a project directory" message the issue names.
func TestInitVerifyClientAnchorsOnTargetDir(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()
	os.Remove(filepath.Join(os.Getenv("HOME"), ".wk", "profiles.json"))
	os.Remove(filepath.Join(os.Getenv("HOME"), ".wk", "active_profile"))

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	targetDir := filepath.Join(dir, "fs-verify")
	os.MkdirAll(targetDir, 0755)
	body := "NAME=e2e\nWORKSPACE=acme\nENVIRONMENT=dev\nREGION=us\nSTORE_TYPE=file\nTOKEN=tok\n"
	if err := os.WriteFile(auth.NewFileStore(targetDir).Path, []byte(body), 0600); err != nil {
		t.Fatalf("seeding profiles.env: %v", err)
	}

	// resolveProfileAndCredForInit is the function --verify uses. It must
	// resolve against targetDir, not CWD (CWD here is `dir`, which has no
	// wk.toml — the pre-fix fileStoreForCwd would fail here).
	resetGlobalFlags(t)
	flagStoreType = string(auth.StoreFile)
	profile, cred, err := resolveProfileAndCredForInit(nil, "e2e", targetDir)
	if err != nil {
		t.Fatalf("resolveProfileAndCredForInit: %v", err)
	}
	if profile == nil || cred == nil {
		t.Fatalf("profile=%v cred=%v, want both non-nil", profile, cred)
	}
	if profile.Name != "e2e" {
		t.Errorf("profile.Name = %q, want e2e", profile.Name)
	}
	if cred.Token != "tok" {
		t.Errorf("cred.Token = %q, want tok", cred.Token)
	}
}

// Deferred mode: --store-type file with no profiles.env at targetDir. init
// continues without a resolved profile; snapshot fields are omitted. No
// --verify here — with --verify this case errors naturally since there are
// no credentials to build a client with.
func TestInitDeferredModeWhenProfilesEnvMissing(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "deferred-proj", "--profile", "e2e", "--store-type", "file", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(filepath.Join(dir, "deferred-proj")))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Workspace != "" {
		t.Errorf("Workspace = %q, want empty (deferred mode leaves snapshot fields unset)", cfg.Workspace)
	}
	if cfg.Profile != "e2e" {
		t.Errorf("Profile = %q, want e2e", cfg.Profile)
	}
}

// Profile truly missing from both keychain and target's profiles.env: error
// should name the expected file-store path so the developer knows where
// the CLI looked.
func TestInitProfileNotFoundNamesExpectedPath(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()
	os.Remove(filepath.Join(os.Getenv("HOME"), ".wk", "profiles.json"))
	os.Remove(filepath.Join(os.Getenv("HOME"), ".wk", "active_profile"))

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "missing-proj", "--profile", "ghost", "--json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	expected := filepath.Join(dir, "missing-proj", auth.ProfilesEnvFile)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err = %v, want mention of %q", err, expected)
	}
}

// TestInitInsideSameNameDir verifies fix #57: running `wk init --name X`
// from inside a directory already named X uses cwd as the target instead
// of creating X/X.
func TestInitInsideSameNameDir(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Create and cd into a directory named "my-project".
	projectDir := filepath.Join(dir, "my-project")
	if err := os.Mkdir(projectDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.Chdir(projectDir)

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "my-project", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// wk.toml should be in projectDir, NOT in projectDir/my-project.
	cfg, err := config.Load(config.ProjectConfigPath(projectDir))
	if err != nil {
		t.Fatalf("loading config from projectDir: %v", err)
	}
	if cfg.Name != "my-project" {
		t.Errorf("Name = %q, want my-project", cfg.Name)
	}

	// The nested directory must NOT exist.
	nested := filepath.Join(projectDir, "my-project")
	if _, err := os.Stat(filepath.Join(nested, config.ProjectDir)); err == nil {
		t.Error("nested my-project/my-project/.wk/ should not exist — init should use cwd")
	}
}

// TestInitOverwriteFromInsideExistingProject verifies fix #59: running
// `wk init --overwrite` from inside an existing project (where
// targetDir == projectRoot) should succeed instead of hitting the nesting
// guard.
func TestInitOverwriteFromInsideExistingProject(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Create a project named "my-project" from the parent directory.
	os.Chdir(dir)
	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "my-project", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first init: %v", err)
	}

	// Now cd into my-project and reinit with --overwrite.
	projectDir := filepath.Join(dir, "my-project")
	os.Chdir(projectDir)

	root = NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "my-project", "--profile", "dev", "--overwrite", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("overwrite from inside project should succeed, got: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(projectDir))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Name != "my-project" {
		t.Errorf("Name = %q, want my-project", cfg.Name)
	}
}
