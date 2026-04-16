// Coverage tests for API struct fields and CLI table output.
// See CONTRIBUTING.md for the full workflow when adding or modifying resources.
package api

import (
	"reflect"
	"testing"
)

// jsonTags extracts all json field names from a struct type,
// stripping ",omitempty" and skipping "-" tags.
func jsonTags(t reflect.Type) map[string]bool {
	tags := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		// Strip options like ",omitempty"
		if idx := len(tag); idx > 0 {
			for j, c := range tag {
				if c == ',' {
					tag = tag[:j]
					break
				}
			}
		}
		tags[tag] = true
	}
	return tags
}

// TestStructFieldCoverage verifies that our Go structs capture all known
// fields from the Workato API. When the API adds new fields, add them to
// the expectedFields list below and then add the corresponding struct field.
//
// This is the single source of truth for "what fields should each struct have?"
// If this test fails, either:
//   - A new API field was discovered → add it to both expectedFields AND the struct
//   - A struct field was accidentally removed → restore it
func TestStructFieldCoverage(t *testing.T) {
	tests := []struct {
		name           string
		structType     reflect.Type
		expectedFields []string
	}{
		{
			name:       "Recipe",
			structType: reflect.TypeOf(Recipe{}),
			expectedFields: []string{
				"id", "name", "description", "folder_id",
				"running", "active", "version",
				"created_at", "updated_at",
				"code", "config",
			},
		},
		{
			name:       "Connection",
			structType: reflect.TypeOf(Connection{}),
			expectedFields: []string{
				"id", "name", "application", "folder_id",
				"authorization_status", "authorization_error",
				"created_at", "updated_at",
			},
		},
		{
			name:       "Folder",
			structType: reflect.TypeOf(Folder{}),
			expectedFields: []string{
				"id", "name", "parent_id",
			},
		},
		{
			name:       "APICollection",
			structType: reflect.TypeOf(APICollection{}),
			expectedFields: []string{
				"id", "name", "handle", "version",
				"description", "use_prefix", "project_id",
			},
		},
		{
			name:       "APIEndpoint",
			structType: reflect.TypeOf(APIEndpoint{}),
			expectedFields: []string{
				"id", "name", "api_collection_id", "active",
				"method", "path", "recipe_id",
			},
		},
		{
			name:       "Job",
			structType: reflect.TypeOf(Job{}),
			expectedFields: []string{
				"id", "recipe_id", "status",
				"started_at", "completed_at",
			},
		},
		{
			name:       "Tag",
			structType: reflect.TypeOf(Tag{}),
			expectedFields: []string{
				"handle", "title", "description", "color",
			},
		},
		{
			name:       "WorkspaceUser",
			structType: reflect.TypeOf(WorkspaceUser{}),
			expectedFields: []string{
				"id", "name", "email",
			},
		},
		{
			name:       "AuditLogEntry",
			structType: reflect.TypeOf(AuditLogEntry{}),
			expectedFields: []string{
				"id", "event_type", "timestamp",
				"user", "details",
			},
		},
		{
			name:       "Connector",
			structType: reflect.TypeOf(Connector{}),
			expectedFields: []string{
				"name", "title", "description",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := jsonTags(tt.structType)

			// Check that every expected API field is present in the struct.
			for _, field := range tt.expectedFields {
				if !tags[field] {
					t.Errorf("struct %s is missing expected API field %q — add it to the struct in types.go", tt.name, field)
				}
			}

			// Reverse check: flag struct fields not in the expected list.
			// These aren't failures — they might be intentional extras — but
			// they should be added to expectedFields to keep the list current.
			expected := make(map[string]bool)
			for _, f := range tt.expectedFields {
				expected[f] = true
			}
			for tag := range tags {
				if !expected[tag] {
					t.Errorf("struct %s has field %q not in expectedFields — add it to the test to acknowledge it", tt.name, tag)
				}
			}
		})
	}
}

// TestStructFieldCoverage_TableColumns verifies that list command table
// outputs include a minimum set of "important" fields. Fields that are
// complex objects (code, config, details) or timestamps are excluded
// since they're better consumed via --json.
//
// To use: when adding a new field to a struct, decide if it should appear
// in the table output. If yes, add it here AND to the command's FormatList call.
func TestStructFieldCoverage_TableColumns(t *testing.T) {
	// Maps resource name → minimum fields that SHOULD appear in table output.
	// Update this list when you add columns to a list command.
	requiredTableFields := map[string][]string{
		"Recipe":        {"id", "name", "description", "folder_id", "running", "version"},
		"Connection":    {"id", "name", "application", "folder_id", "authorization_status"},
		"Folder":        {"id", "name", "parent_id"},
		"APICollection": {"id", "name", "handle", "version", "description", "project_id"},
		"APIEndpoint":   {"id", "name", "method", "path", "recipe_id", "api_collection_id", "active"},
		"Tag":           {"handle", "title", "description", "color"},
		"WorkspaceUser": {"id", "name", "email"},
		"AuditLogEntry": {"id", "event_type", "user", "timestamp"},
		"Connector":     {"name", "title", "description"},
	}

	structTypes := map[string]reflect.Type{
		"Recipe":        reflect.TypeOf(Recipe{}),
		"Connection":    reflect.TypeOf(Connection{}),
		"Folder":        reflect.TypeOf(Folder{}),
		"APICollection": reflect.TypeOf(APICollection{}),
		"APIEndpoint":   reflect.TypeOf(APIEndpoint{}),
		"Tag":           reflect.TypeOf(Tag{}),
		"WorkspaceUser": reflect.TypeOf(WorkspaceUser{}),
		"AuditLogEntry": reflect.TypeOf(AuditLogEntry{}),
		"Connector":     reflect.TypeOf(Connector{}),
	}

	for name, fields := range requiredTableFields {
		t.Run(name+"_table_fields_exist_in_struct", func(t *testing.T) {
			st, ok := structTypes[name]
			if !ok {
				t.Fatalf("no struct type registered for %s", name)
			}
			tags := jsonTags(st)
			for _, field := range fields {
				if !tags[field] {
					t.Errorf("table requires field %q but struct %s doesn't have it — the table output will break", field, name)
				}
			}
		})
	}
}
