package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/workato-devs/wk-cli-beta/internal/config"
	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

// PullResult describes what happened to a single file during pull.
type PullResult struct {
	FilePath string `json:"file_path"`
	// Action is one of: "created", "updated", "unchanged", "skipped",
	// "deleted" (server-side delete reconciled locally), or "orphaned"
	// (server-side deleted but local copy was modified — kept on disk).
	Action string `json:"action"`
}

// Pull downloads remote assets to the local project directory.
// If force is false, it aborts when a locally modified file would be
// overwritten by the fresh export — but only if that file is still present
// server-side. Files that were modified locally AND removed server-side
// are NOT conflicts: reconcileDeletions reports them as "orphaned" and
// leaves them on disk untouched. That narrower semantic means the
// overwrite check must run after the zip is in hand, not before.
func (e *SyncEngine) Pull(entry config.SyncEntry, force bool) ([]PullResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	localDir := filepath.Join(e.projectRoot, entry.LocalPath)

	// Resolve folder ID (cached when possible, ADR-005 Decision 9).
	folderID, err := e.folderIDForEntry(ctx, entry)
	if err != nil {
		return nil, e.wrapFolderErr(err, entry, entry.FolderID)
	}

	// Trigger export; invalidate cache and retry once on 404.
	origFolderID := folderID
	retried := false
	pkgID, err := e.packages.Export(ctx, folderID)
	if err != nil && entry.FolderID != 0 && invalidFolderCacheErr(err) {
		if fresh, rerr := e.resolveAndCache(ctx, entry.ServerPath); rerr == nil {
			folderID = fresh
			retried = true
			pkgID, err = e.packages.Export(ctx, folderID)
		} else {
			return nil, e.wrapFolderErr(rerr, entry, origFolderID)
		}
	}
	if err != nil {
		wrapID := origFolderID
		if retried {
			wrapID = 0 // retry used fresh ID; don't blame the stale cache
		}
		return nil, e.wrapFolderErr(err, entry, wrapID)
	}

	// Wait for export to complete.
	if err := e.waitForPackage(ctx, pkgID); err != nil {
		return nil, err
	}

	// Download the package zip.
	zipData, err := e.packages.Download(ctx, pkgID)
	if err != nil {
		return nil, fmt.Errorf("downloading package: %w", err)
	}

	// Pre-flight overwrite check: abort only when a locally modified file
	// would be clobbered by the fresh zip. Modified files NOT in the zip
	// are orphans and handled by reconcileDeletions below — they don't
	// warrant --force.
	if !force {
		seenPaths, err := zipAssetPaths(zipData)
		if err != nil {
			return nil, fmt.Errorf("inspecting zip for conflicts: %w", err)
		}
		if conflicts, err := e.overwriteConflicts(localDir, seenPaths); err != nil {
			return nil, err
		} else if len(conflicts) > 0 {
			return nil, fmt.Errorf("%w: %s has local modifications and would be overwritten (use --force to overwrite)", wkerrors.ErrSyncConflict, conflicts[0])
		}
	}

	// Extract and write files.
	results, seen, err := e.extractZip(zipData, localDir, entry.ServerPath)
	if err != nil {
		return nil, err
	}

	// Reconcile server-side deletions: any meta file whose asset path is
	// NOT in the fresh zip has been removed from the workspace. Safe to
	// delete locally iff the on-disk file still matches the stored hash.
	// Locally modified files are reported as "orphaned" and left alone.
	delResults, err := e.reconcileDeletions(localDir, seen)
	if err != nil {
		return nil, err
	}
	results = append(results, delResults...)
	return results, nil
}

// extractZip extracts a package zip into localDir, creating/updating meta
// files. Returns the PullResult list and a set of asset paths (relative to
// localDir, native separator) that the zip contained — the caller uses the
// set to reconcile server-side deletions against existing meta files.
func (e *SyncEngine) extractZip(zipData []byte, localDir string, serverPath string) ([]PullResult, map[string]bool, error) {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, nil, fmt.Errorf("opening zip: %w", err)
	}

	// Ensure local directory exists.
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("creating directory %s: %w", localDir, err)
	}

	ignore := e.ignoreMatcher()

	var results []PullResult
	seen := make(map[string]bool)

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("opening %s in zip: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("reading %s in zip: %w", f.Name, err)
		}

		// Normalize zip path.
		relPath := filepath.ToSlash(f.Name)
		relPath = strings.TrimPrefix(relPath, "/")

		// Track in the seen-set using the native-separator form so it
		// lines up with FindMetaFiles keys, which come from filepath.Rel.
		assetRel := filepath.FromSlash(relPath)
		seen[assetRel] = true

		// Consult .wkignore using the project-root-relative path.
		absPath := filepath.Join(localDir, assetRel)
		if projectRel, rerr := e.projectRel(absPath); rerr == nil {
			if ignore.ShouldSkip(projectRel, false) {
				results = append(results, PullResult{FilePath: relPath, Action: "skipped"})
				continue
			}
		}

		// Normalize JSON to prevent phantom diffs from server-side reformatting.
		if isJSON(relPath) {
			if normalized, err := normalizeJSON(data); err == nil {
				data = normalized
			}
		}

		newHash := ComputeHash(data)

		// Determine action.
		action := "created"
		if existing, err := os.ReadFile(absPath); err == nil {
			if ComputeHash(existing) == newHash {
				action = "unchanged"
			} else {
				action = "updated"
			}
		}

		if action != "unchanged" {
			// Ensure parent directory exists.
			if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
				return nil, nil, fmt.Errorf("creating directory for %s: %w", absPath, err)
			}
			if err := os.WriteFile(absPath, data, 0644); err != nil {
				return nil, nil, fmt.Errorf("writing %s: %w", absPath, err)
			}
		}

		// Write/update sidecar meta under .wk/.
		meta := &AssetMeta{
			ServerPath:   serverPath + "/" + relPath,
			ZipName:      f.Name,
			Folder:       filepath.Dir(relPath),
			Type:         inferAssetType(relPath),
			ContentHash:  newHash,
			LastPulledAt: time.Now().UTC(),
		}
		metaPath, err := MetaPath(e.projectRoot, absPath)
		if err != nil {
			return nil, nil, fmt.Errorf("meta path for %s: %w", relPath, err)
		}
		if err := WriteMeta(metaPath, meta); err != nil {
			return nil, nil, fmt.Errorf("writing meta for %s: %w", relPath, err)
		}

		results = append(results, PullResult{
			FilePath: relPath,
			Action:   action,
		})
	}

	return results, seen, nil
}

// zipAssetPaths returns the set of asset paths (relative to the zip root,
// native-separator form) present in zipData. It reads only headers — no
// file contents — so it's cheap to call before extraction.
func zipAssetPaths(zipData []byte) (map[string]bool, error) {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("opening zip: %w", err)
	}
	seen := make(map[string]bool, len(r.File))
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := filepath.ToSlash(f.Name)
		rel = strings.TrimPrefix(rel, "/")
		seen[filepath.FromSlash(rel)] = true
	}
	return seen, nil
}

// overwriteConflicts returns the relative paths of locally modified files
// whose asset is still present server-side (in seenPaths). These are real
// overwrite conflicts — the extract step would clobber the local change.
// Modified files NOT in seenPaths are orphans and are intentionally
// excluded so the user doesn't need --force to accept their removal.
func (e *SyncEngine) overwriteConflicts(localDir string, seenPaths map[string]bool) ([]string, error) {
	metas, err := FindMetaFiles(e.projectRoot, localDir)
	if err != nil {
		return nil, fmt.Errorf("scanning metas for conflict check: %w", err)
	}
	if len(metas) == 0 {
		return nil, nil
	}
	ignore := e.ignoreMatcher()

	var conflicts []string
	for rel, meta := range metas {
		if !seenPaths[rel] {
			continue // orphan — handled by reconcileDeletions
		}
		absPath := filepath.Join(localDir, rel)
		if projectRel, rerr := e.projectRel(absPath); rerr == nil {
			if ignore.ShouldSkip(projectRel, false) {
				continue
			}
		}
		data, readErr := os.ReadFile(absPath)
		if os.IsNotExist(readErr) {
			continue // nothing to lose
		}
		if readErr != nil {
			return nil, fmt.Errorf("reading %s for conflict check: %w", absPath, readErr)
		}
		if ComputeHash(data) != meta.ContentHash {
			conflicts = append(conflicts, filepath.ToSlash(rel))
		}
	}
	return conflicts, nil
}

// reconcileDeletions walks the meta tree for localDir and, for any meta
// whose asset path did not appear in the fresh zip (seen), decides the
// correct action:
//
//   - local file unmodified (hash still matches meta) → delete file + meta,
//     emit "deleted".
//   - local file modified (hash differs) → leave file alone, remove nothing,
//     emit "orphaned" so the developer knows it no longer tracks anything
//     server-side.
//   - local file already missing (only a stale meta remains) → remove the
//     meta silently-ish; emit "deleted" so the output isn't surprisingly blank.
//
// .wkignore rules still apply — ignored metas are not reconciled so the
// developer doesn't see churn after adding an ignore rule.
func (e *SyncEngine) reconcileDeletions(localDir string, seen map[string]bool) ([]PullResult, error) {
	metas, err := FindMetaFiles(e.projectRoot, localDir)
	if err != nil {
		return nil, fmt.Errorf("scanning metas for deletion reconciliation: %w", err)
	}
	if len(metas) == 0 {
		return nil, nil
	}

	ignore := e.ignoreMatcher()

	var results []PullResult
	for rel, meta := range metas {
		if seen[rel] {
			continue
		}
		absPath := filepath.Join(localDir, rel)
		if projectRel, rerr := e.projectRel(absPath); rerr == nil {
			if ignore.ShouldSkip(projectRel, false) {
				continue
			}
		}

		// Resolve the meta path once; we either remove it or leave it alone.
		metaPath, mperr := MetaPath(e.projectRoot, absPath)
		if mperr != nil {
			return nil, fmt.Errorf("meta path for %s: %w", rel, mperr)
		}

		data, readErr := os.ReadFile(absPath)
		reportPath := filepath.ToSlash(rel)
		switch {
		case os.IsNotExist(readErr):
			// Asset already gone; drop the stale meta to keep .wk/ tidy.
			if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("removing stale meta %s: %w", metaPath, err)
			}
			results = append(results, PullResult{FilePath: reportPath, Action: "deleted"})
		case readErr != nil:
			return nil, fmt.Errorf("reading %s: %w", absPath, readErr)
		default:
			if ComputeHash(data) == meta.ContentHash {
				// Unmodified — safe to remove both.
				if err := os.Remove(absPath); err != nil {
					return nil, fmt.Errorf("removing %s: %w", absPath, err)
				}
				if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("removing meta %s: %w", metaPath, err)
				}
				results = append(results, PullResult{FilePath: reportPath, Action: "deleted"})
			} else {
				// Modified locally; leave the file and meta in place so
				// the developer can inspect. Treated as an orphan — no
				// longer syncs to anything server-side.
				results = append(results, PullResult{FilePath: reportPath, Action: "orphaned"})
			}
		}
	}
	return results, nil
}

// isJSON returns true if the file path has a .json extension.
func isJSON(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".json")
}

// normalizeJSON re-serializes JSON with sorted keys and consistent indentation.
func normalizeJSON(data []byte) ([]byte, error) {
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// inferAssetType guesses the asset type from a filename.
func inferAssetType(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "recipe"):
		return "recipe"
	case strings.Contains(lower, "connection"):
		return "connection"
	case strings.HasSuffix(lower, ".api_endpoint.json"):
		return "api_endpoint"
	case strings.HasSuffix(lower, ".api_group.json"):
		return "api_collection"
	default:
		return "unknown"
	}
}
