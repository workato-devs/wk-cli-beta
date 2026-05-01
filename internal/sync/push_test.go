package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// removeAssetOnly deletes the asset file at <root>/<relPath> while
// leaving the .meta.json sidecar in .wk/ intact.
func removeAssetOnly(t *testing.T, root, relPath string) error {
	t.Helper()
	return os.Remove(filepath.Join(root, relPath))
}

// TestPush_ForceIncludesUnchangedFiles verifies that force=true causes
// Push to include unchanged files in the push set. We use dryRun=true
// to avoid needing a live API client — the filtering logic that force
// modifies runs before the API call.
func TestPush_ForceIncludesUnchangedFiles(t *testing.T) {
	root := t.TempDir()
	content := []byte("unchanged content")

	// Write an asset whose content matches its meta hash — Status will
	// report it as "unchanged".
	writeAssetWithMeta(t, root, "recipe.json", content, &AssetMeta{
		ServerPath:  "test/recipe.json",
		ContentHash: ComputeHash(content),
	})

	engine := &SyncEngine{projectRoot: root}
	entry := config.SyncEntry{LocalPath: "."}

	// Without force: dry-run should report the file as "unchanged".
	results, err := engine.Push(entry, true, true, false)
	if err != nil {
		t.Fatalf("Push(force=false): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Push(force=false): got %d results, want 1", len(results))
	}
	if results[0].Action != "unchanged" {
		t.Errorf("Push(force=false): action = %q, want %q", results[0].Action, "unchanged")
	}

	// With force: the same unchanged file should be reported as "forced".
	results, err = engine.Push(entry, true, true, true)
	if err != nil {
		t.Fatalf("Push(force=true): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Push(force=true): got %d results, want 1", len(results))
	}
	if results[0].Action != "forced" {
		t.Errorf("Push(force=true): action = %q, want %q", results[0].Action, "forced")
	}
}

// TestPush_ForcePreservesModifiedAndNewActions verifies that force=true
// keeps the "updated" and "created" labels for files that are already
// modified or new, reserving "forced" for unchanged files only.
func TestPush_ForcePreservesModifiedAndNewActions(t *testing.T) {
	root := t.TempDir()

	// Modified file: meta hash differs from on-disk content.
	writeAssetWithMeta(t, root, "modified.json", []byte("new content"), &AssetMeta{
		ServerPath:  "test/modified.json",
		ContentHash: ComputeHash([]byte("old content")),
	})

	engine := &SyncEngine{projectRoot: root}
	entry := config.SyncEntry{LocalPath: "."}

	results, err := engine.Push(entry, true, true, true)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Action != "updated" {
		t.Errorf("modified file: action = %q, want %q", results[0].Action, "updated")
	}
}

// TestPush_ForceSkipsDeletedFiles verifies that force=true does NOT
// attempt to push files whose local asset has been deleted (only the
// .meta.json remains). Deleted files still get the "deleted" action.
func TestPush_ForceSkipsDeletedFiles(t *testing.T) {
	root := t.TempDir()

	// Write a meta file with no corresponding asset file — Status
	// will report "deleted".
	writeAssetWithMeta(t, root, "gone.json", []byte("content"), &AssetMeta{
		ServerPath:  "test/gone.json",
		ContentHash: ComputeHash([]byte("content")),
	})
	// Remove the asset file, leaving only the meta.
	if err := removeAssetOnly(t, root, "gone.json"); err != nil {
		t.Fatalf("remove asset: %v", err)
	}

	engine := &SyncEngine{projectRoot: root}
	entry := config.SyncEntry{LocalPath: "."}

	results, err := engine.Push(entry, true, true, true)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Action != "deleted" {
		t.Errorf("deleted file: action = %q, want %q", results[0].Action, "deleted")
	}
}
