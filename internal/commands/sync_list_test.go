package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/workato-devs/wk-cli-beta/internal/config"
	"github.com/workato-devs/wk-cli-beta/internal/sync"
)

func TestSyncList_DefaultSmoke(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack", FolderID: 42},
		{ServerPath: "Recipes/GitHub", LocalPath: "./github"},
	})

	var out bytes.Buffer
	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
	// formatter writes to os.Stdout, not the cobra out — sanity-check that
	// list is read-only.
	reloaded, _ := config.Load(config.ProjectConfigPath(cwd))
	if len(reloaded.Sync) != 2 {
		t.Errorf("list should not mutate config: got %+v", reloaded.Sync)
	}
}

// TestSyncList_JSONShape verifies the --json payload structure and the
// per-field serialization rules: FolderID omitted when 0 (omitempty),
// LastSyncedAt omitted when nil (pointer).
func TestSyncList_JSONShape(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "./slack", FolderID: 42},
		{ServerPath: "Recipes/GitHub", LocalPath: "./github"},
	})

	// The formatter writes directly to os.Stdout, so we redirect the fd to
	// capture the JSON payload produced by the command.
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "list", "--json"})
	if err := root.Execute(); err != nil {
		w.Close()
		t.Fatalf("list: %v", err)
	}
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	raw := buf.Bytes()

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("parse json: %v (raw=%s)", err, string(raw))
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2 (raw=%s)", len(rows), string(raw))
	}
	for _, row := range rows {
		if row["server_path"] == "Recipes/Slack" {
			if row["folder_id"] != float64(42) {
				t.Errorf("cached folder_id = %v, want 42", row["folder_id"])
			}
		}
		if row["server_path"] == "Recipes/GitHub" {
			if _, ok := row["folder_id"]; ok {
				t.Errorf("uncached entry should omit folder_id, got %v", row["folder_id"])
			}
		}
	}
}

// TestSyncList_JSONVerboseIncludesLastSynced regression-guards the bug
// where --json --verbose silently dropped last_synced_at because a
// locally-declared --verbose flag shadowed the persistent root flag.
// The helper writes a sidecar meta under .wk/ and asserts the emitted
// JSON row carries the timestamp.
func TestSyncList_JSONVerboseIncludesLastSynced(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes/Slack", LocalPath: "slack"},
	})

	// Drop an asset + sidecar meta so latestLastSynced has something to read.
	assetAbs := filepath.Join(cwd, "slack", "r.json")
	if err := os.MkdirAll(filepath.Dir(assetAbs), 0755); err != nil {
		t.Fatalf("mkdir asset: %v", err)
	}
	if err := os.WriteFile(assetAbs, []byte("x"), 0644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	metaPath, err := sync.MetaPath(cwd, assetAbs)
	if err != nil {
		t.Fatalf("meta path: %v", err)
	}
	fixed := time.Date(2026, 4, 20, 15, 30, 0, 0, time.UTC)
	if err := sync.WriteMeta(metaPath, &sync.AssetMeta{
		ServerPath:   "Recipes/Slack",
		Type:         "recipe",
		ContentHash:  "deadbeef",
		LastPulledAt: fixed,
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "list", "--json", "--verbose"})
	if err := root.Execute(); err != nil {
		w.Close()
		t.Fatalf("list: %v", err)
	}
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	raw := buf.Bytes()

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("parse json: %v (raw=%s)", err, string(raw))
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1 (raw=%s)", len(rows), string(raw))
	}
	got, ok := rows[0]["last_synced_at"].(string)
	if !ok || got == "" {
		t.Fatalf("last_synced_at missing in --json --verbose output (raw=%s)", string(raw))
	}
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("last_synced_at %q not RFC3339: %v", got, err)
	}
	if !parsed.Equal(fixed) {
		t.Errorf("last_synced_at = %v, want %v", parsed, fixed)
	}
}
