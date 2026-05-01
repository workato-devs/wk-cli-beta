package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONFormat(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	data := map[string]string{"name": "test", "version": "1.0"}
	if err := f.Format(&buf, data); err != nil {
		t.Fatalf("Format: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["name"] != "test" {
		t.Errorf("name = %q, want %q", result["name"], "test")
	}
}

func TestJSONFormatList(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	headers := []string{"id", "name"}
	rows := [][]string{{"1", "alpha"}, {"2", "beta"}}

	if err := f.FormatList(&buf, headers, rows); err != nil {
		t.Fatalf("FormatList: %v", err)
	}

	var result []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0]["name"] != "alpha" {
		t.Errorf("result[0][name] = %q, want %q", result[0]["name"], "alpha")
	}
}

func TestTextFormatList(t *testing.T) {
	f := &TextFormatter{}
	var buf bytes.Buffer

	headers := []string{"ID", "NAME"}
	rows := [][]string{{"1", "alpha"}, {"2", "beta"}}

	if err := f.FormatList(&buf, headers, rows); err != nil {
		t.Fatalf("FormatList: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ID") || !strings.Contains(output, "NAME") {
		t.Errorf("output missing headers: %s", output)
	}
	if !strings.Contains(output, "alpha") || !strings.Contains(output, "beta") {
		t.Errorf("output missing data: %s", output)
	}
}

func TestJSONFormatStructSlice(t *testing.T) {
	type item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	f := &JSONFormatter{}
	var buf bytes.Buffer

	items := []item{{ID: 42, Name: "alpha"}, {ID: 7, Name: "beta"}}
	if err := f.Format(&buf, items); err != nil {
		t.Fatalf("Format: %v", err)
	}

	var result []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	// Verify lowercase json tag keys are used
	if _, ok := result[0]["id"]; !ok {
		t.Error("expected lowercase key 'id'")
	}
	if _, ok := result[0]["ID"]; ok {
		t.Error("unexpected uppercase key 'ID'")
	}
	// Verify numeric types are preserved (json.Number or float64)
	idVal, ok := result[0]["id"].(float64)
	if !ok {
		t.Fatalf("id should be numeric, got %T", result[0]["id"])
	}
	if idVal != 42 {
		t.Errorf("id = %v, want 42", idVal)
	}
	if result[0]["name"] != "alpha" {
		t.Errorf("name = %v, want alpha", result[0]["name"])
	}
}

func TestJSONFormatPage_UsesNativeStructs(t *testing.T) {
	type recipe struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	f := &JSONFormatter{}
	var buf bytes.Buffer

	data := []recipe{{ID: 1, Name: "alpha"}, {ID: 2, Name: "beta"}}
	headers := []string{"ID", "NAME"}
	rows := [][]string{{"1", "alpha"}, {"2", "beta"}}
	meta := PageMeta{Page: 1, PerPage: 10, HasNext: true}

	if err := f.FormatPage(&buf, data, headers, rows, meta); err != nil {
		t.Fatalf("FormatPage: %v", err)
	}

	var result struct {
		Items   []map[string]any `json:"items"`
		Page    int              `json:"page"`
		PerPage int              `json:"per_page"`
		HasNext bool             `json:"has_next"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Page != 1 {
		t.Errorf("page = %d, want 1", result.Page)
	}
	if result.PerPage != 10 {
		t.Errorf("per_page = %d, want 10", result.PerPage)
	}
	if !result.HasNext {
		t.Error("has_next = false, want true")
	}
	if len(result.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(result.Items))
	}
	if _, ok := result.Items[0]["id"]; !ok {
		t.Error("expected lowercase key 'id' from struct tag")
	}
	if _, ok := result.Items[0]["ID"]; ok {
		t.Error("unexpected uppercase key 'ID' — should use struct json tags")
	}
	idVal, ok := result.Items[0]["id"].(float64)
	if !ok {
		t.Fatalf("id should be numeric, got %T", result.Items[0]["id"])
	}
	if idVal != 1 {
		t.Errorf("id = %v, want 1", idVal)
	}
}

func TestJSONFormatPage_NoNext(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	meta := PageMeta{Page: 3, PerPage: 5, HasNext: false}
	if err := f.FormatPage(&buf, []string{}, nil, nil, meta); err != nil {
		t.Fatalf("FormatPage: %v", err)
	}

	var result struct {
		HasNext bool `json:"has_next"`
		Page    int  `json:"page"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.HasNext {
		t.Error("has_next = true, want false")
	}
	if result.Page != 3 {
		t.Errorf("page = %d, want 3", result.Page)
	}
}

func TestTextFormatPage_DelegatesToFormatList(t *testing.T) {
	f := &TextFormatter{}
	var buf bytes.Buffer

	headers := []string{"ID", "NAME"}
	rows := [][]string{{"1", "alpha"}}
	meta := PageMeta{Page: 1, PerPage: 10, HasNext: false}

	if err := f.FormatPage(&buf, nil, headers, rows, meta); err != nil {
		t.Fatalf("FormatPage: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Errorf("output missing headers: %s", out)
	}
	if !strings.Contains(out, "alpha") {
		t.Errorf("output missing data: %s", out)
	}
}

func TestNewFormatter(t *testing.T) {
	jf := NewFormatter("json")
	if _, ok := jf.(*JSONFormatter); !ok {
		t.Error("NewFormatter(json) should return JSONFormatter")
	}

	tf := NewFormatter("text")
	if _, ok := tf.(*TextFormatter); !ok {
		t.Error("NewFormatter(text) should return TextFormatter")
	}

	df := NewFormatter("")
	if _, ok := df.(*TextFormatter); !ok {
		t.Error("NewFormatter('') should default to TextFormatter")
	}
}
