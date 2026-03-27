package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// pollInterval is the delay between status checks when waiting for
// server-side export/import operations.
const pollInterval = 2 * time.Second

// implicitRootFolder is the Workato UI label for the workspace root.
// It does not correspond to an actual API folder.
const implicitRootFolder = "All projects"

// resolveFolderID walks the Workato folder hierarchy to find the folder
// matching serverPath (e.g. "Recipes/Production/Integrations").
// The special name "All projects" is treated as the implicit workspace root
// and stripped from the path before resolution.
func (e *SyncEngine) resolveFolderID(ctx context.Context, serverPath string) (int, error) {
	parts := strings.Split(strings.Trim(serverPath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("empty server path")
	}

	// Strip the implicit root folder if present.
	if strings.EqualFold(parts[0], implicitRootFolder) {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return 0, nil
	}

	var parentID *int
	for _, name := range parts {
		folders, err := e.folders.List(ctx, parentID)
		if err != nil {
			return 0, fmt.Errorf("listing folders under %v: %w", parentID, err)
		}
		found := false
		for _, f := range folders {
			if strings.EqualFold(f.Name, name) {
				id := f.ID
				parentID = &id
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("folder %q not found under parent %v", name, parentID)
		}
	}
	return *parentID, nil
}

// waitForPackage polls the export status until the package is complete.
func (e *SyncEngine) waitForPackage(ctx context.Context, pkgID int) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for export (package %d): %w", pkgID, ctx.Err())
		default:
		}

		pkg, err := e.packages.ExportStatus(ctx, pkgID)
		if err != nil {
			return fmt.Errorf("checking export status: %w", err)
		}

		switch pkg.Status {
		case "completed", "succeeded":
			return nil
		case "failed", "error":
			msg := fmt.Sprintf("export failed (package %d): status %s", pkgID, pkg.Status)
			if pkg.Error != "" {
				msg += ": " + pkg.Error
			}
			if len(pkg.ErrorParts) > 0 {
				msg += fmt.Sprintf(" (details: %v)", pkg.ErrorParts)
			}
			return fmt.Errorf("%s", msg)
		}

		time.Sleep(pollInterval)
	}
}

// waitForImport polls the import status until the import is complete.
func (e *SyncEngine) waitForImport(ctx context.Context, importID int) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for import %d: %w", importID, ctx.Err())
		default:
		}

		pkg, err := e.packages.ImportStatus(ctx, importID)
		if err != nil {
			return fmt.Errorf("checking import status: %w", err)
		}

		switch pkg.Status {
		case "completed", "succeeded":
			return nil
		case "failed", "error":
			msg := fmt.Sprintf("import failed (import %d): status %s", importID, pkg.Status)
			if pkg.Error != "" {
				msg += ": " + pkg.Error
			}
			if len(pkg.ErrorParts) > 0 {
				msg += fmt.Sprintf(" (details: %v)", pkg.ErrorParts)
			}
			return fmt.Errorf("%s", msg)
		}

		time.Sleep(pollInterval)
	}
}

// readFileBytes reads a file and returns its contents.
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// findLocalFiles walks a directory and returns all non-meta file paths
// relative to that directory.
func findLocalFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || IsMetaFile(info.Name()) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}
