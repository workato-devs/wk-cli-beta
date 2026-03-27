package sync

import "testing"

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
