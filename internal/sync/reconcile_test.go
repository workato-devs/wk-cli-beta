package sync

import (
	"os"
	"path/filepath"
	"testing"
)

// findResult returns the first PullResult whose FilePath matches (native
// or slash separators both accepted) and whose action is expected. It fails
// the test if no match is found.
func findResult(t *testing.T, results []PullResult, file, action string) {
	t.Helper()
	wantSlash := filepath.ToSlash(file)
	for _, r := range results {
		if filepath.ToSlash(r.FilePath) == wantSlash && r.Action == action {
			return
		}
	}
	t.Fatalf("expected result %q action=%q in %+v", file, action, results)
}

// setupReconcileRoot makes a project root with a .wkignore file ready.
func setupReconcileRoot(t *testing.T, ignoreLines string) *SyncEngine {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".wk"), 0755); err != nil {
		t.Fatalf("mkdir .wk: %v", err)
	}
	if ignoreLines != "" {
		if err := os.WriteFile(filepath.Join(root, ".wkignore"), []byte(ignoreLines), 0644); err != nil {
			t.Fatalf("write .wkignore: %v", err)
		}
	}
	return &SyncEngine{projectRoot: root}
}

func TestReconcileDeletions_UnmodifiedAssetRemoved(t *testing.T) {
	engine := setupReconcileRoot(t, "")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")
	content := []byte("v1 content\n")

	writeAssetWithMeta(t, root, "recipes/slack.json", content, &AssetMeta{
		ServerPath:  "Recipes/slack.json",
		ContentHash: ComputeHash(content),
	})

	// Seen map is empty — the asset was removed server-side.
	results, err := engine.reconcileDeletions(localDir, map[string]bool{})
	if err != nil {
		t.Fatalf("reconcileDeletions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d (%+v)", len(results), results)
	}
	findResult(t, results, "slack.json", "deleted")

	if _, err := os.Stat(filepath.Join(localDir, "slack.json")); !os.IsNotExist(err) {
		t.Errorf("asset should be deleted, stat err = %v", err)
	}
	metaPath, _ := MetaPath(root, filepath.Join(localDir, "slack.json"))
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Errorf("meta should be deleted, stat err = %v", err)
	}
}

func TestReconcileDeletions_ModifiedAssetOrphanedNotDeleted(t *testing.T) {
	engine := setupReconcileRoot(t, "")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")
	stored := []byte("original\n")
	modified := []byte("locally edited\n")

	// Meta reflects the original hash, but the on-disk file has diverged.
	writeAssetWithMeta(t, root, "recipes/slack.json", modified, &AssetMeta{
		ServerPath:  "Recipes/slack.json",
		ContentHash: ComputeHash(stored),
	})

	results, err := engine.reconcileDeletions(localDir, map[string]bool{})
	if err != nil {
		t.Fatalf("reconcileDeletions: %v", err)
	}
	findResult(t, results, "slack.json", "orphaned")

	// Both the asset and its meta must still be on disk.
	if _, err := os.Stat(filepath.Join(localDir, "slack.json")); err != nil {
		t.Errorf("modified asset should not be deleted: %v", err)
	}
	metaPath, _ := MetaPath(root, filepath.Join(localDir, "slack.json"))
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("meta for orphaned asset should stay: %v", err)
	}
}

func TestReconcileDeletions_AssetMissingMetaIsCleaned(t *testing.T) {
	engine := setupReconcileRoot(t, "")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")

	// Only the meta exists — asset was already gone before this pull.
	metaPath, err := MetaPath(root, filepath.Join(localDir, "gone.json"))
	if err != nil {
		t.Fatalf("MetaPath: %v", err)
	}
	if err := WriteMeta(metaPath, &AssetMeta{ServerPath: "x/gone.json"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	results, err := engine.reconcileDeletions(localDir, map[string]bool{})
	if err != nil {
		t.Fatalf("reconcileDeletions: %v", err)
	}
	findResult(t, results, "gone.json", "deleted")

	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Errorf("stale meta should be removed, stat err = %v", err)
	}
}

func TestReconcileDeletions_SeenAssetUntouched(t *testing.T) {
	engine := setupReconcileRoot(t, "")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")
	content := []byte("v1\n")

	writeAssetWithMeta(t, root, "recipes/slack.json", content, &AssetMeta{
		ServerPath:  "Recipes/slack.json",
		ContentHash: ComputeHash(content),
	})

	seen := map[string]bool{"slack.json": true}
	results, err := engine.reconcileDeletions(localDir, seen)
	if err != nil {
		t.Fatalf("reconcileDeletions: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("seen asset should produce no reconciliation results, got %+v", results)
	}
	if _, err := os.Stat(filepath.Join(localDir, "slack.json")); err != nil {
		t.Errorf("seen asset should remain on disk: %v", err)
	}
}

func TestReconcileDeletions_IgnoredAssetSkipped(t *testing.T) {
	engine := setupReconcileRoot(t, "recipes/**\n")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")
	content := []byte("v1\n")

	writeAssetWithMeta(t, root, "recipes/ignored.json", content, &AssetMeta{
		ServerPath:  "Recipes/ignored.json",
		ContentHash: ComputeHash(content),
	})

	results, err := engine.reconcileDeletions(localDir, map[string]bool{})
	if err != nil {
		t.Fatalf("reconcileDeletions: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ignored meta should not be reconciled, got %+v", results)
	}
	if _, err := os.Stat(filepath.Join(localDir, "ignored.json")); err != nil {
		t.Errorf("ignored asset should remain on disk: %v", err)
	}
}

func TestReconcileDeletions_NoMetasReturnsNil(t *testing.T) {
	engine := setupReconcileRoot(t, "")
	root := engine.projectRoot
	localDir := filepath.Join(root, "empty")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	results, err := engine.reconcileDeletions(localDir, map[string]bool{})
	if err != nil {
		t.Fatalf("reconcileDeletions: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results, got %+v", results)
	}
}

// A modified local file whose asset is still present server-side is a
// real overwrite conflict — extract would clobber the edit.
func TestOverwriteConflicts_ModifiedAndPresent(t *testing.T) {
	engine := setupReconcileRoot(t, "")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")
	stored := []byte("v1\n")
	edited := []byte("v1 + local edit\n")
	writeAssetWithMeta(t, root, "recipes/slack.json", edited, &AssetMeta{
		ServerPath:  "Recipes/slack.json",
		ContentHash: ComputeHash(stored),
	})
	seen := map[string]bool{"slack.json": true}
	conflicts, err := engine.overwriteConflicts(localDir, seen)
	if err != nil {
		t.Fatalf("overwriteConflicts: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0] != "slack.json" {
		t.Errorf("conflicts = %+v, want [slack.json]", conflicts)
	}
}

// A modified local file whose asset was removed server-side is an
// orphan — NOT a conflict. It will be reported by reconcileDeletions
// as "orphaned" and left on disk.
func TestOverwriteConflicts_ModifiedAndOrphan_NotAConflict(t *testing.T) {
	engine := setupReconcileRoot(t, "")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")
	stored := []byte("v1\n")
	edited := []byte("v1 + local edit\n")
	writeAssetWithMeta(t, root, "recipes/slack.json", edited, &AssetMeta{
		ServerPath:  "Recipes/slack.json",
		ContentHash: ComputeHash(stored),
	})
	// seen is empty — server has deleted the asset.
	conflicts, err := engine.overwriteConflicts(localDir, map[string]bool{})
	if err != nil {
		t.Fatalf("overwriteConflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("modified orphans must not register as conflicts, got %+v", conflicts)
	}
}

// An unmodified local file is never a conflict even if it's still in
// the zip — extraction will be a no-op for identical bytes.
func TestOverwriteConflicts_UnmodifiedIsNotAConflict(t *testing.T) {
	engine := setupReconcileRoot(t, "")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")
	content := []byte("v1\n")
	writeAssetWithMeta(t, root, "recipes/slack.json", content, &AssetMeta{
		ServerPath:  "Recipes/slack.json",
		ContentHash: ComputeHash(content),
	})
	conflicts, err := engine.overwriteConflicts(localDir, map[string]bool{"slack.json": true})
	if err != nil {
		t.Fatalf("overwriteConflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("unmodified file must not register as a conflict, got %+v", conflicts)
	}
}

func TestOverwriteConflicts_IgnoredFileSkipped(t *testing.T) {
	engine := setupReconcileRoot(t, "recipes/**\n")
	root := engine.projectRoot
	localDir := filepath.Join(root, "recipes")
	stored := []byte("v1\n")
	edited := []byte("edited\n")
	writeAssetWithMeta(t, root, "recipes/slack.json", edited, &AssetMeta{
		ServerPath:  "Recipes/slack.json",
		ContentHash: ComputeHash(stored),
	})
	conflicts, err := engine.overwriteConflicts(localDir, map[string]bool{"slack.json": true})
	if err != nil {
		t.Fatalf("overwriteConflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("ignored file must not register as a conflict, got %+v", conflicts)
	}
}
