package commands

import (
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

func TestResolveSyncEntries(t *testing.T) {
	entry1 := config.SyncEntry{ServerPath: "Home/Recipes", LocalPath: "recipes"}
	entry2 := config.SyncEntry{ServerPath: "Home/Connections", LocalPath: "connections"}

	tests := []struct {
		name      string
		cfg       *config.Config
		folder    string
		wantCount int
		wantErr   bool
	}{
		{
			name:    "empty sync returns error",
			cfg:     &config.Config{},
			folder:  "",
			wantErr: true,
		},
		{
			name:      "no filter returns all entries",
			cfg:       &config.Config{Sync: []config.SyncEntry{entry1, entry2}},
			folder:    "",
			wantCount: 2,
		},
		{
			name:      "filter by server_path returns matching entry",
			cfg:       &config.Config{Sync: []config.SyncEntry{entry1, entry2}},
			folder:    "Home/Connections",
			wantCount: 1,
		},
		{
			name:      "filter by local_path returns matching entry",
			cfg:       &config.Config{Sync: []config.SyncEntry{entry1, entry2}},
			folder:    "recipes",
			wantCount: 1,
		},
		{
			name:    "no match returns error",
			cfg:     &config.Config{Sync: []config.SyncEntry{entry1, entry2}},
			folder:  "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := resolveSyncEntries(tt.cfg, tt.folder)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entries) != tt.wantCount {
				t.Errorf("got %d entries, want %d", len(entries), tt.wantCount)
			}
		})
	}
}
