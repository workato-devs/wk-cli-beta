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
			cfg, err := config.Load(filepath.Join(dir, "test-proj", config.ProjectFile))
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
	cfg, err := config.Load(filepath.Join(projectDir, config.ProjectFile))
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

	cfg, err := config.Load(filepath.Join(projectDir, config.ProjectFile))
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

	// Create a project directory with wk.toml already inside.
	projectDir := filepath.Join(dir, "existing")
	os.Mkdir(projectDir, 0755)
	config.Save(filepath.Join(projectDir, config.ProjectFile), &config.Config{Name: "existing"})

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

	// Create a wk.toml in the temp dir to simulate being inside a project.
	config.Save(filepath.Join(dir, config.ProjectFile), &config.Config{Name: "parent"})

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

	cfg, err := config.Load(filepath.Join(dir, "snap-project", config.ProjectFile))
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
	cfg, err := config.Load(filepath.Join(dir, "file-proj", config.ProjectFile))
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

	// Pre-create the target directory and a profiles.env in it.
	projectDir := filepath.Join(dir, "ci-proj")
	if err := os.Mkdir(projectDir, 0755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	profilesEnv := "NAME=ci\nWORKSPACE=acme\nENVIRONMENT=prod\nREGION=us\nTOKEN=secret\n"
	if err := os.WriteFile(filepath.Join(projectDir, auth.ProfilesEnvFile), []byte(profilesEnv), 0600); err != nil {
		t.Fatalf("writing profiles.env: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newInitCmd())
	root.SetArgs([]string{"init", "--name", "ci-proj", "--profile", "ci", "--store-type", "file", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	cfg, err := config.Load(filepath.Join(projectDir, config.ProjectFile))
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
