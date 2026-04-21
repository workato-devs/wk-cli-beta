package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/config"
	"github.com/workato-devs/wk-cli-beta/internal/sync"
)

// seedRecipeLocals writes a .recipe.json + .meta.json sidecar pair for the given
// recipe name under localDir inside cwd. Uses a name-keyed filename (the
// shape pull actually writes) and sets meta.RecipeName so the match path
// exercises the production code path end-to-end.
func seedRecipeLocals(t *testing.T, cwd, localSub, recipeName string) (string, string) {
	t.Helper()
	localDir := filepath.Join(cwd, localSub)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}
	fname := recipeName + ".recipe.json"
	assetPath := filepath.Join(localDir, fname)
	if err := os.WriteFile(assetPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	metaPath, err := sync.MetaPath(cwd, assetPath)
	if err != nil {
		t.Fatalf("MetaPath: %v", err)
	}
	if err := sync.WriteMeta(metaPath, &sync.AssetMeta{
		Type:       "recipe",
		ZipName:    fname,
		ServerPath: "Recipes/" + recipeName,
		RecipeName: recipeName,
	}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	return assetPath, metaPath
}

func TestCleanupLocalRecipeFiles_RemovesPair(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)

	writeProjectSkel(t, cwd, []config.SyncEntry{{ServerPath: "Recipes", LocalPath: "./recipes"}})
	assetPath, metaPath := seedRecipeLocals(t, cwd, "recipes", "slack_bot")

	removed := cleanupLocalRecipeFiles("slack_bot")
	if len(removed) != 2 {
		t.Fatalf("removed = %v, want 2 paths", removed)
	}
	if _, err := os.Stat(assetPath); !os.IsNotExist(err) {
		t.Errorf("asset should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Errorf("meta should be gone, stat err = %v", err)
	}
}

func TestCleanupLocalRecipeFiles_OutsideProjectNoop(t *testing.T) {
	resetGlobalFlags(t)
	setupIsolatedHome(t) // cwd has no wk.toml
	removed := cleanupLocalRecipeFiles("anything")
	if len(removed) != 0 {
		t.Errorf("cleanupLocalRecipeFiles outside project should be a no-op, got %v", removed)
	}
}

func TestCleanupLocalRecipeFiles_MismatchedNameLeavesPair(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{{ServerPath: "Recipes", LocalPath: "./recipes"}})
	assetPath, metaPath := seedRecipeLocals(t, cwd, "recipes", "slack_bot")

	removed := cleanupLocalRecipeFiles("unrelated_name")
	if len(removed) != 0 {
		t.Errorf("different recipe name should not match, got %v", removed)
	}
	if _, err := os.Stat(assetPath); err != nil {
		t.Errorf("asset should remain: %v", err)
	}
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("meta should remain: %v", err)
	}
}

func TestCleanupLocalRecipeFiles_EmptyNameNoop(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectSkel(t, cwd, []config.SyncEntry{{ServerPath: "Recipes", LocalPath: "./recipes"}})
	seedRecipeLocals(t, cwd, "recipes", "slack_bot")

	if removed := cleanupLocalRecipeFiles(""); len(removed) != 0 {
		t.Errorf("empty name must not match any meta, got %v", removed)
	}
}

func TestMetaMatchesRecipe(t *testing.T) {
	cases := []struct {
		name       string
		meta       *sync.AssetMeta
		recipeName string
		want       bool
	}{
		{"exact name match", &sync.AssetMeta{Type: "recipe", RecipeName: "slack_bot"}, "slack_bot", true},
		{"name mismatch", &sync.AssetMeta{Type: "recipe", RecipeName: "slack_bot"}, "other_bot", false},
		{"legacy meta without recipe_name", &sync.AssetMeta{Type: "recipe"}, "slack_bot", false},
		{"wrong type", &sync.AssetMeta{Type: "connection", RecipeName: "slack_bot"}, "slack_bot", false},
		{"nil meta", nil, "slack_bot", false},
		{"empty query name", &sync.AssetMeta{Type: "recipe", RecipeName: "slack_bot"}, "", false},
		{"both empty does not match", &sync.AssetMeta{Type: "recipe"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := metaMatchesRecipe(tc.meta, tc.recipeName)
			if got != tc.want {
				t.Errorf("metaMatchesRecipe(%v, %q) = %v, want %v", tc.meta, tc.recipeName, got, tc.want)
			}
		})
	}
}
