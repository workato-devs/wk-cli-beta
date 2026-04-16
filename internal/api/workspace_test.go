package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkspaceService_GetCurrentUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/users/me" {
			t.Errorf("path = %s, want /users/me", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WorkspaceUser{ID: 1, Name: "Alice", Email: "alice@example.com"})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	user, err := client.Workspace().GetCurrentUser(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Name != "Alice" {
		t.Errorf("name = %q, want Alice", user.Name)
	}
}

func TestWorkspaceService_ListMembers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("email") != "bob@example.com" {
			t.Errorf("email = %q, want bob@example.com", r.URL.Query().Get("email"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
				"data": []WorkspaceUser{{ID: 2, Name: "Bob", Email: "bob@example.com"}},
			})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	members, err := client.Workspace().ListMembers(context.Background(), "bob@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 1 || members[0].Name != "Bob" {
		t.Errorf("got %+v, want 1 member named Bob", members)
	}
}

func TestWorkspaceService_GetAuditLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/activity_logs" {
			t.Errorf("path = %s, want /activity_logs", r.URL.Path)
		}
		if r.URL.Query().Get("from") != "2026-01-01" {
			t.Errorf("from = %q, want 2026-01-01", r.URL.Query().Get("from"))
		}
		if r.URL.Query().Get("event_type") != "recipe_started" {
			t.Errorf("event_type = %q, want recipe_started", r.URL.Query().Get("event_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
				"data": []AuditLogEntry{{ID: 1, EventType: "recipe_started", User: struct {
						ID    int    `json:"id"`
						Name  string `json:"name"`
						Email string `json:"email"`
					}{ID: 10}}},
			})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	entries, err := client.Workspace().GetAuditLogs(context.Background(), &AuditLogOptions{
		Since:  "2026-01-01",
		Action: "recipe_started",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].EventType != "recipe_started" {
		t.Errorf("got %+v, want 1 entry", entries)
	}
}
