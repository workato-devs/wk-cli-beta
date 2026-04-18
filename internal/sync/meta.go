package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// AssetMeta is the sidecar metadata stored for each synced asset.
//
// Per ADR-005 Decision 1, metas live under the project's .wk/ mirror
// tree — the asset at <root>/<rel> has its meta at
// <root>/.wk/<rel>.meta.json. Meta files never sit next to assets.
//
// RecipeName is populated only when Type == "recipe" and the JSON body
// in the pull zip contained a parsable top-level "name" field. The
// Workato package-manifest zip intentionally omits server-side IDs
// (the wk recipes export endpoint is the only source for those), so
// downstream local-cleanup paths (e.g. wk recipes delete) match by
// name — resolving an ID to a name via a single API call first.
type AssetMeta struct {
	ServerPath   string    `json:"server_path"`
	ZipName      string    `json:"zip_name"`
	Folder       string    `json:"folder"`
	Type         string    `json:"type"` // "recipe", "connection"
	Version      int       `json:"version"`
	RecipeName   string    `json:"recipe_name,omitempty"`
	ContentHash  string    `json:"content_hash"` // SHA256
	LastPulledAt time.Time `json:"last_pulled_at"`
}

// metaSuffix is appended to the asset's relative path to form the meta
// filename inside .wk/. Example: recipes/slack.recipe.json ->
// .wk/recipes/slack.recipe.json.meta.json.
const metaSuffix = ".meta.json"

// MetaPath returns the canonical meta path for an asset located at
// assetAbs within the project rooted at projectRoot. The meta lives
// inside projectRoot/.wk/ mirroring the asset's relative position.
func MetaPath(projectRoot, assetAbs string) (string, error) {
	rel, err := filepath.Rel(projectRoot, assetAbs)
	if err != nil {
		return "", fmt.Errorf("computing rel path for %s: %w", assetAbs, err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("asset %s is outside project root %s", assetAbs, projectRoot)
	}
	return filepath.Join(projectRoot, config.ProjectDir, rel+metaSuffix), nil
}

// ReadMeta reads and unmarshals a meta file at the given path.
func ReadMeta(metaPath string) (*AssetMeta, error) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("reading meta %s: %w", metaPath, err)
	}
	var meta AssetMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing meta %s: %w", metaPath, err)
	}
	return &meta, nil
}

// WriteMeta marshals and writes a meta file, creating parent directories
// under .wk/ as needed.
func WriteMeta(metaPath string, meta *AssetMeta) error {
	if err := os.MkdirAll(filepath.Dir(metaPath), 0755); err != nil {
		return fmt.Errorf("creating meta dir for %s: %w", metaPath, err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding meta: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(metaPath, data, 0644)
}

// ComputeHash returns the SHA256 hex digest of data.
func ComputeHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// FindMetaFiles scans the .wk/ mirror tree for metas describing assets
// whose asset path is under localDir. Keys in the returned map are asset
// paths relative to localDir, so callers can compare them directly
// against filepath.Walk rel-paths of the asset tree.
func FindMetaFiles(projectRoot, localDir string) (map[string]*AssetMeta, error) {
	result := make(map[string]*AssetMeta)

	// The meta subtree corresponding to localDir lives at
	// <projectRoot>/.wk/<rel-of-localDir>/.
	localRel, err := filepath.Rel(projectRoot, localDir)
	if err != nil {
		return nil, fmt.Errorf("computing rel path for %s: %w", localDir, err)
	}
	metaRoot := filepath.Join(projectRoot, config.ProjectDir, localRel)

	info, err := os.Stat(metaRoot)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", metaRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", metaRoot)
	}

	err = filepath.Walk(metaRoot, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(fi.Name(), metaSuffix) {
			return nil
		}

		meta, err := ReadMeta(path)
		if err != nil {
			return err
		}

		// Recover the asset path relative to localDir by stripping
		// metaRoot prefix and the metaSuffix.
		relMeta, err := filepath.Rel(metaRoot, path)
		if err != nil {
			return err
		}
		assetRel := strings.TrimSuffix(relMeta, metaSuffix)
		result[assetRel] = meta
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %s for meta files: %w", metaRoot, err)
	}
	return result, nil
}
