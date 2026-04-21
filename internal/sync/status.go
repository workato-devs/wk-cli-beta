package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// FileStatus describes the local state of a synced asset.
type FileStatus string

const (
	StatusUnchanged FileStatus = "unchanged"
	StatusModified  FileStatus = "modified"
	StatusNew       FileStatus = "new"     // local file with no .meta.json sidecar in .wk/
	StatusDeleted   FileStatus = "deleted" // .meta.json exists in .wk/ but asset file is gone
)

// AssetStatus represents the sync status of a single asset file.
type AssetStatus struct {
	FilePath   string     `json:"file_path"`
	Status     FileStatus `json:"status"`
	ServerPath string     `json:"server_path,omitempty"` // from meta, if available
}

// Status computes the local modification state for a sync entry.
// This is a purely local operation -- no API calls are made.
func (e *SyncEngine) Status(entry config.SyncEntry) ([]AssetStatus, error) {
	localDir := filepath.Join(e.projectRoot, entry.LocalPath)

	// Ensure the directory exists.
	info, err := os.Stat(localDir)
	if os.IsNotExist(err) {
		return nil, nil // nothing synced yet
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", localDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", localDir)
	}

	// Load all meta files to know what was previously synced.
	metas, err := FindMetaFiles(e.projectRoot, localDir)
	if err != nil {
		return nil, err
	}

	ignore := e.ignoreMatcher()

	// Track which metas we've matched to an actual file.
	matched := make(map[string]bool)

	var results []AssetStatus

	// Walk all files in the sync directory.
	err = filepath.Walk(localDir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		projectRel, rerr := e.projectRel(path)
		if rerr != nil {
			return rerr
		}
		if fi.IsDir() {
			// Skip the .wk/ tool directory (ADR-005 Decision 10).
			if fi.Name() == config.ProjectDir {
				return filepath.SkipDir
			}
			if ignore.ShouldSkip(projectRel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignore.ShouldSkip(projectRel, false) {
			return nil
		}

		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}

		meta, hasMeta := metas[rel]
		if !hasMeta {
			// File exists locally but has no meta -- it's new.
			results = append(results, AssetStatus{
				FilePath: rel,
				Status:   StatusNew,
			})
			return nil
		}

		matched[rel] = true

		// Compare current content hash with stored hash.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		currentHash := ComputeHash(data)

		status := StatusUnchanged
		if currentHash != meta.ContentHash {
			status = StatusModified
		}

		results = append(results, AssetStatus{
			FilePath:   rel,
			Status:     status,
			ServerPath: meta.ServerPath,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", localDir, err)
	}

	// Any meta file with no corresponding asset file means deleted.
	// Skip metas for paths the user has since ignored so the user doesn't
	// see spurious "deleted" entries after adding a .wkignore rule.
	for rel, meta := range metas {
		if matched[rel] {
			continue
		}
		assetAbs := filepath.Join(localDir, rel)
		if projectRel, err := e.projectRel(assetAbs); err == nil {
			if ignore.ShouldSkip(projectRel, false) {
				continue
			}
		}
		results = append(results, AssetStatus{
			FilePath:   rel,
			Status:     StatusDeleted,
			ServerPath: meta.ServerPath,
		})
	}

	return results, nil
}
