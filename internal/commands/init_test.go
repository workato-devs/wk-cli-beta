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

func TestInitLocalPathDefaults(t *testing.T) {
	tests := []struct {
		name       string
		serverPath string
		localPath  string
		wantLocal  string
	}{
		{
			name:       "defaults to dot",
			serverPath: "Test",
			localPath:  "",
			wantLocal:  ".",
		},
		{
			name:       "nested server path defaults to dot",
			serverPath: "Dev Team Testing/Gong.io API",
			localPath:  "",
			wantLocal:  ".",
		},
		{
			name:       "explicit local path preserved",
			serverPath: "Test",
			localPath:  "./custom",
			wantLocal:  "./custom",
		},
		{
			name:       "trailing slash defaults to dot",
			serverPath: "Test/Sub/",
			localPath:  "",
			wantLocal:  ".",
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

			args := []string{
				"--name", "test-proj",
				"--profile", "dev",
				"--server-path", tt.serverPath,
				"--json",
			}
			if tt.localPath != "" {
				args = append(args, "--local-path", tt.localPath)
			}

			root := NewRootCmd()
			root.AddCommand(newInitCmd())
			root.SetArgs(append([]string{"init"}, args...))
			if err := root.Execute(); err != nil {
				t.Fatalf("init failed: %v", err)
			}

			// Init creates a container directory: <cwd>/test-proj/wk.toml
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

	// Pre-create the target .wk/ directory and drop a profiles.env in it.
	// profiles.env lives at <projectRoot>/.wk/profiles.env per ADR-006
	// Sub-decision 3 (alongside wk.toml) so that .wk/.gitignore automatically
	// prevents accidental commits of the secrets file.
	projectDir := filepath.Join(dir, "ci-proj")
	if err := os.MkdirAll(filepath.Join(projectDir, config.ProjectDir), 0755); err != nil {
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

	// Project-root .gitignore must NOT be created — developer owns that file.
	rootIgnore := filepath.Join(dir, "gitigproj", ".gitignore")
	if _, err := os.Stat(rootIgnore); !os.IsNotExist(err) {
		t.Errorf("project-root .gitignore should not exist, got err=%v", err)
	}
}

func TestInitDoesNotModifyProjectRootGitignore(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Pre-create target with a developer-owned .gitignore unrelated to wk.
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

	// Project-root .gitignore should be byte-for-byte unchanged.
	got, err := os.ReadFile(rootIgnore)
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if string(got) != preExisting {
		t.Errorf("project-root .gitignore was modified.\nwant: %q\ngot:  %q", preExisting, got)
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

func TestInitOverwritePreservesSyncEntries(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// First init creates the project.
	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "multiproj", "--profile", "dev", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Hand-edit wk.toml to add multiple sync entries — this mirrors the
	// current workaround for issue #29 (single-entry limit on init).
	configPath := config.ProjectConfigPath(filepath.Join(dir, "multiproj"))
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	cfg.Sync = []config.SyncEntry{
		{ServerPath: "Team/A", LocalPath: "a"},
		{ServerPath: "Team/B", LocalPath: "b"},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("saving edited config: %v", err)
	}

	// Re-init with --overwrite — without preservation this would drop both entries.
	root = NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "multiproj", "--profile", "dev", "--overwrite", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("overwrite init failed: %v", err)
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	if len(reloaded.Sync) != 2 {
		t.Fatalf("sync entries after overwrite = %d, want 2 (should have been preserved)", len(reloaded.Sync))
	}
	paths := map[string]bool{}
	for _, s := range reloaded.Sync {
		paths[s.ServerPath] = true
	}
	if !paths["Team/A"] || !paths["Team/B"] {
		t.Errorf("preserved entries missing: %+v", reloaded.Sync)
	}
}

func TestInitOverwriteAppendsNewServerPath(t *testing.T) {
	cleanupHome := setupTestHome(t)
	defer cleanupHome()

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// First init with one --server-path.
	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "addproj", "--profile", "dev", "--server-path", "Team/A", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Re-init with --overwrite and a different --server-path — should keep A and add B.
	root = NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "addproj", "--profile", "dev", "--overwrite", "--server-path", "Team/B", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("overwrite init failed: %v", err)
	}

	cfg, err := config.Load(config.ProjectConfigPath(filepath.Join(dir, "addproj")))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Sync) != 2 {
		t.Fatalf("sync entries = %d, want 2 (original preserved + new appended): %+v", len(cfg.Sync), cfg.Sync)
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
