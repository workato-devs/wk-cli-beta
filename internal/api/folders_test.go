package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFolderService_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if pid := r.URL.Query().Get("parent_id"); pid != "10" {
			t.Errorf("parent_id = %q, want 10", pid)
		}
		w.Header().Set("Content-Type", "application/json")
		// Production expects raw array (no wrapper).
		json.NewEncoder(w).Encode([]Folder{{ID: 1, Name: "child"}})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	pid := 10
	folders, err := client.Folders().List(context.Background(), &pid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(folders) != 1 || folders[0].Name != "child" {
		t.Errorf("got %+v, want 1 folder named child", folders)
	}
}

func TestFolderService_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "new-folder" {
			t.Errorf("name = %v, want new-folder", body["name"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Folder{ID: 5, Name: "new-folder"})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	folder, err := client.Folders().Create(context.Background(), "new-folder", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder.ID != 5 {
		t.Errorf("ID = %d, want 5", folder.ID)
	}
}

func TestFolderService_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/folders/7" {
			t.Errorf("path = %s, want /folders/7", r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	err := client.Folders().Delete(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
