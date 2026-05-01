package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// PushResult describes what happened to a single file during push.
type PushResult struct {
	FilePath string `json:"file_path"`
	Action   string `json:"action"` // "created", "updated", "deleted", "unchanged" (dry-run)
}

// Push uploads local assets to the remote workspace.
// If dryRun is true, it reports what would be pushed without making changes.
// preserveState controls whether recipe active state is preserved on import.
// If force is true, all tracked files are pushed regardless of local change status.
func (e *SyncEngine) Push(entry config.SyncEntry, dryRun bool, preserveState bool, force bool) ([]PushResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	localDir := filepath.Join(e.projectRoot, entry.LocalPath)

	// Get current status to know what to push.
	statuses, err := e.Status(entry)
	if err != nil {
		return nil, fmt.Errorf("checking local status: %w", err)
	}

	// Filter to files that have changes (or all non-deleted files when force is set).
	var toPush []AssetStatus
	var results []PushResult
	for _, s := range statuses {
		if force && s.Status != StatusDeleted {
			toPush = append(toPush, s)
			action := "forced"
			if s.Status == StatusNew {
				action = "created"
			} else if s.Status == StatusModified {
				action = "updated"
			}
			results = append(results, PushResult{
				FilePath: s.FilePath,
				Action:   action,
			})
			continue
		}
		switch s.Status {
		case StatusModified, StatusNew:
			toPush = append(toPush, s)
			action := "updated"
			if s.Status == StatusNew {
				action = "created"
			}
			results = append(results, PushResult{
				FilePath: s.FilePath,
				Action:   action,
			})
		case StatusDeleted:
			results = append(results, PushResult{
				FilePath: s.FilePath,
				Action:   "deleted",
			})
		case StatusUnchanged:
			if dryRun {
				results = append(results, PushResult{
					FilePath: s.FilePath,
					Action:   "unchanged",
				})
			}
		}
	}

	if dryRun {
		return results, nil
	}

	if len(toPush) == 0 {
		return results, nil
	}

	// Build a zip from local files.
	zipData, err := e.buildZip(localDir, toPush)
	if err != nil {
		return nil, fmt.Errorf("building zip: %w", err)
	}

	// Resolve folder ID (cached when possible, ADR-005 Decision 9).
	folderID, err := e.folderIDForEntry(ctx, entry)
	if err != nil {
		return nil, e.wrapFolderErr(err, entry, entry.FolderID)
	}

	// Upload; invalidate cache and retry once on 404. The retry predicate
	// checks the fresh folderID (not entry.FolderID) because the entry
	// comes in by value — when folderIDForEntry freshly resolves or
	// creates a folder, the updated ID lives on e.config.Sync[i] and
	// on folderID, not on the local copy. Using entry.FolderID here
	// would silently skip the retry for any just-resolved entry (and
	// the create branch makes this case common enough to matter).
	origFolderID := folderID
	retried := false
	restartRecipes := preserveState
	importID, err := e.packages.Import(ctx, folderID, zipData, restartRecipes)
	if err != nil && folderID != 0 && invalidFolderCacheErr(err) {
		if fresh, rerr := e.resolveAndCache(ctx, entry.ServerPath); rerr == nil {
			folderID = fresh
			retried = true
			importID, err = e.packages.Import(ctx, folderID, zipData, restartRecipes)
		} else {
			return nil, e.wrapFolderErr(rerr, entry, origFolderID)
		}
	}
	if err != nil {
		wrapID := origFolderID
		if retried {
			wrapID = 0
		}
		return nil, e.wrapFolderErr(err, entry, wrapID)
	}

	// Wait for import to complete.
	if err := e.waitForImport(ctx, importID); err != nil {
		return nil, err
	}

	// Update meta files for pushed assets (under .wk/).
	for _, s := range toPush {
		absPath := filepath.Join(localDir, s.FilePath)
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		metaPath, err := MetaPath(e.projectRoot, absPath)
		if err != nil {
			continue
		}
		meta, _ := ReadMeta(metaPath)
		if meta == nil {
			meta = &AssetMeta{
				ServerPath: entry.ServerPath + "/" + s.FilePath,
				ZipName:    s.FilePath,
				Folder:     filepath.Dir(s.FilePath),
				Type:       inferAssetType(s.FilePath),
			}
		}
		meta.ContentHash = ComputeHash(data)
		meta.LastPulledAt = time.Now().UTC()
		_ = WriteMeta(metaPath, meta)
	}

	return results, nil
}

// buildZip creates a zip archive from the given local asset files.
// .wkignore filtering already happened in Status(); this walk trusts the
// passed list but still honors the implicit .wk/ skip if somehow present.
func (e *SyncEngine) buildZip(localDir string, assets []AssetStatus) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	ignore := e.ignoreMatcher()

	for _, s := range assets {
		absPath := filepath.Join(localDir, s.FilePath)
		if projectRel, rerr := e.projectRel(absPath); rerr == nil {
			if ignore.ShouldSkip(projectRel, false) {
				continue
			}
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", s.FilePath, err)
		}

		// Use the zip_name from meta if available, otherwise use the relative path.
		zipName := s.FilePath
		if metaPath, err := MetaPath(e.projectRoot, absPath); err == nil {
			if meta, err := ReadMeta(metaPath); err == nil && meta.ZipName != "" {
				zipName = meta.ZipName
			}
		}

		f, err := w.Create(zipName)
		if err != nil {
			return nil, fmt.Errorf("creating zip entry %s: %w", zipName, err)
		}
		if _, err := f.Write(data); err != nil {
			return nil, fmt.Errorf("writing zip entry %s: %w", zipName, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing zip: %w", err)
	}
	return buf.Bytes(), nil
}
