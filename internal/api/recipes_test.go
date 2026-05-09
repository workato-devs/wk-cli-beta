package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecipeService_ListJobs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/recipes/42/jobs" {
			t.Errorf("path = %s, want /recipes/42/jobs", r.URL.Path)
		}
		if s := r.URL.Query().Get("status"); s != "succeeded" {
			t.Errorf("status = %q, want succeeded", s)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListResult[Job]{Items: []Job{{ID: "j-1", RecipeID: 42, Status: "succeeded"}}})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	jobs, err := client.Recipes().ListJobs(context.Background(), 42, &JobListOptions{Status: "succeeded"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Status != "succeeded" {
		t.Errorf("got %+v, want 1 succeeded job", jobs)
	}
}

func TestRecipeService_GetJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/recipes/42/jobs/j-abc123" {
			t.Errorf("path = %s, want /recipes/42/jobs/j-abc123", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"id": "j-abc123", "recipe_id": 42, "status": "failed",
			"is_error": true, "error": "Connection timeout",
			"handle": "j-abc123",
			"lines": [
				{"recipe_line_number": 1, "adapter_name": "rest", "adapter_operation": "make_request_v2"},
				{"recipe_line_number": 2, "adapter_name": "logger", "adapter_operation": "log_message"}
			]
		}`
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	detail, err := client.Recipes().GetJob(context.Background(), 42, "j-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Status != "failed" {
		t.Errorf("status = %q, want failed", detail.Status)
	}
	if detail.Error == nil || *detail.Error != "Connection timeout" {
		t.Errorf("error = %v, want 'Connection timeout'", detail.Error)
	}
	if detail.Handle != "j-abc123" {
		t.Errorf("handle = %q, want j-abc123", detail.Handle)
	}
	if len(detail.Lines) != 2 {
		t.Fatalf("lines count = %d, want 2", len(detail.Lines))
	}
	if detail.Lines[0].AdapterName != "rest" {
		t.Errorf("lines[0].adapter_name = %q, want rest", detail.Lines[0].AdapterName)
	}
}

func TestRecipeService_Copy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/recipes/42/copy" {
			t.Errorf("path = %s, want /recipes/42/copy", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["folder_id"] != float64(100) {
			t.Errorf("folder_id = %v, want 100", body["folder_id"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Recipe{ID: 99, Name: "copy", FolderID: 100})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	recipe, err := client.Recipes().Copy(context.Background(), 42, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recipe.ID != 99 {
		t.Errorf("ID = %d, want 99", recipe.ID)
	}
}

func TestRecipeService_Connect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/recipes/42/connect" {
			t.Errorf("path = %s, want /recipes/42/connect", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["adapter_name"] != "salesforce" {
			t.Errorf("adapter_name = %v, want salesforce", body["adapter_name"])
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	err := client.Recipes().Connect(context.Background(), 42, "salesforce", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecipeService_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/recipes/42" {
			t.Errorf("path = %s, want /recipes/42", r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	if err := client.Recipes().Delete(context.Background(), 42); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestRecipeService_Import_FollowUpGet(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/recipes":
			json.NewDecoder(r.Body).Decode(&captured)
			w.Write([]byte(`{"success":true,"id":206448}`))
		case r.Method == "GET" && r.URL.Path == "/recipes/206448":
			json.NewEncoder(w).Encode(Recipe{ID: 206448, Name: "imported", FolderID: 14116})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	body := []byte(`{"name":"imported","code":{"x":1},"config":[{"k":"v"}]}`)

	client := NewHTTPClient(srv.URL, "test-token")
	recipe, err := client.Recipes().Import(context.Background(), 14116, body)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if recipe.ID != 206448 || recipe.Name != "imported" || recipe.FolderID != 14116 {
		t.Errorf("recipe = %+v, want ID=206448 Name=imported FolderID=14116", recipe)
	}
	if fid, ok := captured["folder_id"].(string); !ok || fid != "14116" {
		t.Errorf("folder_id = %v (%T), want string 14116", captured["folder_id"], captured["folder_id"])
	}
	if s, ok := captured["code"].(string); !ok || s != `{"x":1}` {
		t.Errorf("code not stringified; got %T %v", captured["code"], captured["code"])
	}
}

func TestRecipeService_Update_StringifiesCodeAndConfig(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/recipes/42" {
			t.Errorf("path = %s, want /recipes/42", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	// Export-style JSON: code and config are objects (not pre-stringified)
	// and a stray folder_id exists — Update must stringify the first two
	// and drop the third to match Import's contract.
	body := []byte(`{"name":"r","folder_id":99,"code":{"x":1},"config":[{"k":"v"}]}`)

	client := NewHTTPClient(srv.URL, "test-token")
	if err := client.Recipes().Update(context.Background(), 42, body); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, hasFolder := captured["folder_id"]; hasFolder {
		t.Errorf("folder_id should be stripped on update; body = %+v", captured)
	}
	if s, ok := captured["code"].(string); !ok || s != `{"x":1}` {
		t.Errorf("code not stringified; got %T %v", captured["code"], captured["code"])
	}
	if s, ok := captured["config"].(string); !ok || s != `[{"k":"v"}]` {
		t.Errorf("config not stringified; got %T %v", captured["config"], captured["config"])
	}
}

func TestRecipeService_Import_BackfillsConfigName(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/recipes":
			json.NewDecoder(r.Body).Decode(&captured)
			w.Write([]byte(`{"success":true,"id":1}`))
		case r.Method == "GET" && r.URL.Path == "/recipes/1":
			json.NewEncoder(w).Encode(Recipe{ID: 1, Name: "test", FolderID: 10})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	// Config entry missing "name" — should be backfilled from "provider".
	body := []byte(`{"name":"test","code":{},"config":[{"keyword":"application","provider":"salesforce","account_id":123}]}`)

	client := NewHTTPClient(srv.URL, "test-token")
	_, err := client.Recipes().Import(context.Background(), 10, body)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	configStr, ok := captured["config"].(string)
	if !ok {
		t.Fatalf("config not stringified; got %T", captured["config"])
	}
	var config []map[string]any
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	if len(config) != 1 {
		t.Fatalf("config entries = %d, want 1", len(config))
	}
	if config[0]["name"] != "salesforce" {
		t.Errorf("name = %v, want salesforce (backfilled from provider)", config[0]["name"])
	}
}

func TestRecipeService_Import_PreservesExistingConfigName(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/recipes":
			json.NewDecoder(r.Body).Decode(&captured)
			w.Write([]byte(`{"success":true,"id":1}`))
		case r.Method == "GET" && r.URL.Path == "/recipes/1":
			json.NewEncoder(w).Encode(Recipe{ID: 1, Name: "test", FolderID: 10})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	// Config entry already has "name" — should not be overwritten.
	body := []byte(`{"name":"test","code":{},"config":[{"keyword":"application","provider":"salesforce","name":"salesforce","account_id":123}]}`)

	client := NewHTTPClient(srv.URL, "test-token")
	_, err := client.Recipes().Import(context.Background(), 10, body)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	configStr, ok := captured["config"].(string)
	if !ok {
		t.Fatalf("config not stringified; got %T", captured["config"])
	}
	var config []map[string]any
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	if config[0]["name"] != "salesforce" {
		t.Errorf("name = %v, want salesforce (preserved)", config[0]["name"])
	}
}

func TestRecipeService_ListVersions_DataWrapper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/recipes/42/versions" {
			t.Errorf("path = %s, want /recipes/42/versions", r.URL.Path)
		}
		if per := r.URL.Query().Get("per_page"); per != "50" {
			t.Errorf("per_page = %q, want 50", per)
		}
		w.Header().Set("Content-Type", "application/json")
		// Note the {"data": ...} wrapper, distinct from {"items": ...}
		w.Write([]byte(`{"data":[{"id":1,"version_no":2,"author_name":"Zayne","author_email":"z@x","created_at":"2026-04-13T14:55:25Z","updated_at":"2026-04-13T14:55:25Z"}]}`))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	vs, err := client.Recipes().ListVersions(context.Background(), 42, 0, 50)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(vs) != 1 || vs[0].VersionNo != 2 || vs[0].AuthorName != "Zayne" {
		t.Errorf("versions = %+v, want single entry parsed from data wrapper", vs)
	}
}

func TestRecipeService_GetVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/recipes/42/versions/99" {
			t.Errorf("path = %s, want /recipes/42/versions/99", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":99,"version_no":3}`))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	v, err := client.Recipes().GetVersion(context.Background(), 42, 99)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if v.ID != 99 || v.VersionNo != 3 {
		t.Errorf("version = %+v, want ID=99 VersionNo=3", v)
	}
}

func TestRecipeService_UpdateVersionComment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/recipes/42/versions/99" {
			t.Errorf("path = %s, want /recipes/42/versions/99", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["comment"] != "ok" {
			t.Errorf("comment = %v, want 'ok'", body["comment"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":99,"version_no":3,"comment":"ok"}`))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	v, err := client.Recipes().UpdateVersionComment(context.Background(), 42, 99, "ok")
	if err != nil {
		t.Fatalf("UpdateVersionComment: %v", err)
	}
	if v.Comment == nil || *v.Comment != "ok" {
		t.Errorf("comment roundtrip failed: %+v", v)
	}
}

func TestRecipeService_UpdateVersionComment_TooLong(t *testing.T) {
	client := NewHTTPClient("http://unused", "test-token")
	long := ""
	for i := 0; i < 256; i++ {
		long += "x"
	}
	_, err := client.Recipes().UpdateVersionComment(context.Background(), 42, 99, long)
	if err == nil || !strings.Contains(err.Error(), "255-character limit") {
		t.Errorf("err = %v, want 255-character limit", err)
	}
}
