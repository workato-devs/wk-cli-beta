package sync

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIsJSON(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"recipe.json", true},
		{"folder/recipe.JSON", true},
		{"folder/recipe.Json", true},
		{"recipe.txt", false},
		{"json", false},
		{"folder/data.json.bak", false},
	}
	for _, tt := range tests {
		if got := isJSON(tt.path); got != tt.want {
			t.Errorf("isJSON(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestNormalizeJSON(t *testing.T) {
	// Unsorted keys, inconsistent whitespace.
	input := []byte(`{"z":1,"a":2, "m": {"b":3,"a":4}}`)
	got, err := normalizeJSON(input)
	if err != nil {
		t.Fatalf("normalizeJSON() error: %v", err)
	}
	want := "{\n  \"a\": 2,\n  \"m\": {\n    \"a\": 4,\n    \"b\": 3\n  },\n  \"z\": 1\n}\n"
	if string(got) != want {
		t.Errorf("normalizeJSON() =\n%s\nwant:\n%s", got, want)
	}

	// Idempotency: normalizing the output again should produce the same bytes.
	got2, err := normalizeJSON(got)
	if err != nil {
		t.Fatalf("normalizeJSON(normalized) error: %v", err)
	}
	if string(got2) != string(got) {
		t.Errorf("normalizeJSON is not idempotent:\nfirst:  %s\nsecond: %s", got, got2)
	}
}

func TestNormalizeJSON_InvalidInput(t *testing.T) {
	_, err := normalizeJSON([]byte("not json"))
	if err == nil {
		t.Error("normalizeJSON(non-JSON) should return an error")
	}
}

func TestExtractJSONStringField(t *testing.T) {
	cases := []struct {
		name string
		data string
		key  string
		want string
	}{
		{"top-level name", `{"name": "slack_bot", "version": 3}`, "name", "slack_bot"},
		{"missing key", `{"other": "x"}`, "name", ""},
		{"malformed json", `not json`, "name", ""},
		{"wrong type (number)", `{"name": 123}`, "name", ""},
		{"nested name ignored", `{"meta": {"name": "slack_bot"}}`, "name", ""},
		{"empty object", `{}`, "name", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractJSONStringField([]byte(tc.data), tc.key)
			if got != tc.want {
				t.Errorf("extractJSONStringField(%q, %q) = %q, want %q", tc.data, tc.key, got, tc.want)
			}
		})
	}
}

func TestInferAssetType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"folder/my_recipe.json", "recipe"},
		{"Recipe.json", "recipe"},
		{"folder/connection_salesforce.json", "connection"},
		{"Connection.json", "connection"},
		{"folder/get_users.api_endpoint.json", "api_endpoint"},
		{"GET_users.API_ENDPOINT.JSON", "api_endpoint"},
		{"folder/v1.api_group.json", "api_collection"},
		{"V1.API_GROUP.JSON", "api_collection"},
		{"folder/lookup_table.json", "unknown"},
		{"random.txt", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := inferAssetType(tt.path)
			if got != tt.want {
				t.Errorf("inferAssetType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// buildZipBytes builds an in-memory zip from a map of relative path -> body.
func buildZipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := f.Write(body); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// Post-fix regression: extractZip must populate AssetMeta.RecipeName from
// the top-level "name" field of each recipe JSON. Matching by name (not
// ID) is the correct semantic because IDs are per-environment and
// ephemeral — the same recipe has different IDs in dev and prod — while
// names round-trip cleanly. The Workato package-manifest zip reflects
// this: it carries "name" but not "id".
func TestExtractZip_PopulatesRecipeNameFromJSONBody(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".wk"), 0755); err != nil {
		t.Fatalf("mkdir .wk: %v", err)
	}
	engine := &SyncEngine{projectRoot: root}
	localDir := filepath.Join(root, "recipes")

	// Realistic zip body shape: code, config, description, name, version —
	// no "id" field. This mirrors what wk pull actually writes.
	recipeBody := []byte(`{"name":"slack_bot","description":"bot","code":{"x":1},"config":[],"version":3,"private":false,"concurrency":1}`)
	zipData := buildZipBytes(t, map[string][]byte{
		"slack_bot.recipe.json": recipeBody,
		"notes.txt":             []byte("not a recipe"),
	})

	results, seen, err := engine.extractZip(zipData, localDir, "Recipes/slack")
	if err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	if len(results) == 0 || !seen["slack_bot.recipe.json"] {
		t.Fatalf("extract results unexpected: results=%+v seen=%+v", results, seen)
	}

	metaPath, err := MetaPath(root, filepath.Join(localDir, "slack_bot.recipe.json"))
	if err != nil {
		t.Fatalf("MetaPath: %v", err)
	}
	meta, err := ReadMeta(metaPath)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.RecipeName != "slack_bot" {
		t.Errorf("meta.RecipeName = %q, want %q (extracted from JSON body)", meta.RecipeName, "slack_bot")
	}
}

// TestInferAssetType_KnownWorkatoExtensions is a drift-detection test.
// It lists all known Workato export file extensions and asserts that
// inferAssetType returns a non-"unknown" type for each.
//
// When Workato adds a new export file type:
//  1. Add the extension to this list
//  2. The test will fail because inferAssetType returns "unknown"
//  3. Add a case to inferAssetType in pull.go
//  4. Test passes again
//
// This prevents silent regressions where new asset types get
// type:"unknown" in .wk-meta.json.
func TestInferAssetType_KnownWorkatoExtensions(t *testing.T) {
	knownExtensions := []struct {
		extension    string // compound file extension used by Workato exports
		exampleFile  string // realistic filename
		expectedType string // what inferAssetType should return
	}{
		{".recipe.json", "folder/send_slack_message.recipe.json", "recipe"},
		{".connection.json", "folder/salesforce.connection.json", "connection"},
		{".api_endpoint.json", "folder/get_users.api_endpoint.json", "api_endpoint"},
		{".api_group.json", "folder/v1.api_group.json", "api_collection"},
		// Add new Workato export extensions here as they are discovered.
		// Examples of extensions that may exist but are not yet confirmed:
		// {".lookup_table.json", "folder/states.lookup_table.json", "lookup_table"},
		// {".property.json", "folder/env.property.json", "property"},
		// {".common_data_model.json", "folder/order.common_data_model.json", "common_data_model"},
		// {".recipe_function.json", "folder/helper.recipe_function.json", "recipe_function"},
	}

	for _, ext := range knownExtensions {
		t.Run(ext.extension, func(t *testing.T) {
			got := inferAssetType(ext.exampleFile)
			if got == "unknown" {
				t.Errorf("inferAssetType(%q) = %q, but extension %s is a known Workato type — add a case to inferAssetType()",
					ext.exampleFile, got, ext.extension)
			}
			if got != ext.expectedType {
				t.Errorf("inferAssetType(%q) = %q, want %q", ext.exampleFile, got, ext.expectedType)
			}
		})
	}
}
