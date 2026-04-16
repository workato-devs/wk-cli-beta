package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

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

			// Init now creates a container directory: <cwd>/test-proj/wk.toml
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
