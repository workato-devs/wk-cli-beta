package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// startFolderServer returns an httptest server that responds to
// GET /api/folders with the given folder list. The caller must defer
// srv.Close().
func startFolderServer(t *testing.T, folders []api.Folder) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/folders" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(folders)
	}))
}

// writeDiscoverProject creates .wk/wk.toml with the given sync entries
// and a profiles.env whose BASE_URL points at the test server, so that
// resolveAPIClient succeeds end-to-end.
func writeDiscoverProject(t *testing.T, cwd string, entries []config.SyncEntry, baseURL string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(cwd, config.ProjectDir), 0755); err != nil {
		t.Fatalf("mkdir .wk: %v", err)
	}
	cfg := &config.Config{Name: "discover-test", Profile: "ci", Sync: entries}
	if err := config.Save(config.ProjectConfigPath(cwd), cfg); err != nil {
		t.Fatalf("save cfg: %v", err)
	}
	body := "NAME=ci\nREGION=us\nWORKSPACE=acme\nENVIRONMENT=dev\nBASE_URL=" + baseURL + "\nTOKEN=tok-test\n"
	if err := os.WriteFile(auth.NewFileStore(cwd).Path, []byte(body), 0600); err != nil {
		t.Fatalf("writing profiles.env: %v", err)
	}
}

// TestSyncDiscover_MixedMappedUnmapped verifies the core scenario: some
// server folders are configured in [[sync]], others are not. The command
// should label each correctly.
func TestSyncDiscover_MixedMappedUnmapped(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	flagStoreType = string(auth.StoreFile)
	t.Cleanup(func() { flagStoreType = "" })

	srv := startFolderServer(t, []api.Folder{
		{ID: 1, Name: "Recipes"},
		{ID: 2, Name: "Connections"},
		{ID: 3, Name: "Lookups"},
	})
	defer srv.Close()

	writeDiscoverProject(t, cwd, []config.SyncEntry{
		{ServerPath: "Recipes", LocalPath: "./recipes", FolderID: 1},
	}, srv.URL)

	// Capture stdout (formatter writes to os.Stdout, not cobra's out).
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "discover", "--store-type", "file", "--profile", "ci", "--json"})
	if err := root.Execute(); err != nil {
		w.Close()
		t.Fatalf("discover: %v", err)
	}
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	raw := buf.Bytes()

	var rows []discoverRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("parse json: %v (raw=%s)", err, string(raw))
	}
	if len(rows) != 3 {
		t.Fatalf("rows len = %d, want 3 (raw=%s)", len(rows), string(raw))
	}

	want := map[string]string{
		"Recipes":     "mapped",
		"Connections": "unmapped",
		"Lookups":     "unmapped",
	}
	for _, row := range rows {
		expected, ok := want[row.Folder]
		if !ok {
			t.Errorf("unexpected folder %q in output", row.Folder)
			continue
		}
		if row.Status != expected {
			t.Errorf("folder %q: status = %q, want %q", row.Folder, row.Status, expected)
		}
		if row.Status == "mapped" && row.LocalPath != "./recipes" {
			t.Errorf("folder %q: local_path = %q, want ./recipes", row.Folder, row.LocalPath)
		}
		if row.Status == "unmapped" && row.LocalPath != "" {
			t.Errorf("folder %q: local_path = %q, want empty for unmapped", row.Folder, row.LocalPath)
		}
	}
}

// TestSyncDiscover_AllMapped verifies that when every server folder has a
// matching sync entry, all rows show "mapped".
func TestSyncDiscover_AllMapped(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	flagStoreType = string(auth.StoreFile)
	t.Cleanup(func() { flagStoreType = "" })

	srv := startFolderServer(t, []api.Folder{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
	})
	defer srv.Close()

	writeDiscoverProject(t, cwd, []config.SyncEntry{
		{ServerPath: "Alpha", LocalPath: "./alpha", FolderID: 1},
		{ServerPath: "Beta", LocalPath: "./beta", FolderID: 2},
	}, srv.URL)

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "discover", "--store-type", "file", "--profile", "ci", "--json"})
	if err := root.Execute(); err != nil {
		w.Close()
		t.Fatalf("discover: %v", err)
	}
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var rows []discoverRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("parse json: %v (raw=%s)", err, buf.String())
	}
	for _, row := range rows {
		if row.Status != "mapped" {
			t.Errorf("folder %q: status = %q, want mapped", row.Folder, row.Status)
		}
	}
}

// TestSyncDiscover_CaseInsensitiveMatch verifies that server path
// comparison is case-insensitive — a sync entry with server_path
// "recipes" should match a server folder named "Recipes".
func TestSyncDiscover_CaseInsensitiveMatch(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	flagStoreType = string(auth.StoreFile)
	t.Cleanup(func() { flagStoreType = "" })

	srv := startFolderServer(t, []api.Folder{
		{ID: 1, Name: "Recipes"},
	})
	defer srv.Close()

	writeDiscoverProject(t, cwd, []config.SyncEntry{
		{ServerPath: "recipes", LocalPath: "./recipes"},
	}, srv.URL)

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "discover", "--store-type", "file", "--profile", "ci", "--json"})
	if err := root.Execute(); err != nil {
		w.Close()
		t.Fatalf("discover: %v", err)
	}
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var rows []discoverRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("parse json: %v (raw=%s)", err, buf.String())
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].Status != "mapped" {
		t.Errorf("case-insensitive match failed: status = %q, want mapped", rows[0].Status)
	}
}

// TestSyncDiscover_EmptyWorkspace verifies that an empty folder list
// from the API produces an empty JSON array (no crash).
func TestSyncDiscover_EmptyWorkspace(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	flagStoreType = string(auth.StoreFile)
	t.Cleanup(func() { flagStoreType = "" })

	srv := startFolderServer(t, []api.Folder{})
	defer srv.Close()

	writeDiscoverProject(t, cwd, []config.SyncEntry{
		{ServerPath: "Orphan", LocalPath: "./orphan"},
	}, srv.URL)

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	root := NewRootCmd()
	root.AddCommand(newSyncCmd())
	root.SetArgs([]string{"sync", "discover", "--store-type", "file", "--profile", "ci", "--json"})
	if err := root.Execute(); err != nil {
		w.Close()
		t.Fatalf("discover: %v", err)
	}
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var rows []discoverRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("parse json: %v (raw=%s)", err, buf.String())
	}
	if len(rows) != 0 {
		t.Errorf("rows len = %d, want 0 for empty workspace", len(rows))
	}
}
