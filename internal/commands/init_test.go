package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

func TestInitLocalPathDefaultsToServerPathLeaf(t *testing.T) {
	tests := []struct {
		name       string
		serverPath string
		localPath  string
		wantLocal  string
	}{
		{
			name:       "simple server path",
			serverPath: "Test",
			localPath:  "",
			wantLocal:  "./Test",
		},
		{
			name:       "nested server path uses leaf",
			serverPath: "Dev Team Testing/Gong.io API",
			localPath:  "",
			wantLocal:  "./Gong.io API",
		},
		{
			name:       "explicit local path preserved",
			serverPath: "Test",
			localPath:  "./custom",
			wantLocal:  "./custom",
		},
		{
			name:       "trailing slash stripped",
			serverPath: "Test/Sub/",
			localPath:  "",
			wantLocal:  "./Sub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			origDir, _ := os.Getwd()
			os.Chdir(dir)
			defer os.Chdir(origDir)

			// Build args for init command
			args := []string{
				"--name", "test-proj",
				"--workspace", "dev",
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

			cfg, err := config.Load(filepath.Join(dir, config.ProjectFile))
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
