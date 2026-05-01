package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSkillService_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/agentic/skills" {
			t.Errorf("path = %s, want /agentic/skills", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []Skill{{ID: 1, Name: "my-skill", Description: "test skill", RecipeID: 100, FolderID: 10, ProjectID: 5}},
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	skills, err := client.Skills().List(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "my-skill" {
		t.Errorf("got %+v, want 1 skill named my-skill", skills)
	}
	s := skills[0]
	if s.Description != "test skill" {
		t.Errorf("Description = %q, want %q", s.Description, "test skill")
	}
	if s.RecipeID != 100 {
		t.Errorf("RecipeID = %d, want 100", s.RecipeID)
	}
	if s.FolderID != 10 {
		t.Errorf("FolderID = %d, want 10", s.FolderID)
	}
	if s.ProjectID != 5 {
		t.Errorf("ProjectID = %d, want 5", s.ProjectID)
	}
}

func TestSkillService_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/agentic/skills/42" {
			t.Errorf("path = %s, want /agentic/skills/42", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Skill{ID: 42, Name: "lookup-skill", RecipeID: 200, FolderID: 20, ProjectID: 8})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	s, err := client.Skills().Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != 42 {
		t.Errorf("ID = %d, want 42", s.ID)
	}
	if s.Name != "lookup-skill" {
		t.Errorf("Name = %q, want %q", s.Name, "lookup-skill")
	}
}

func TestSkillService_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/agentic/skills" {
			t.Errorf("path = %s, want /agentic/skills", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["recipe_id"] != float64(300) {
			t.Errorf("recipe_id = %v, want 300", body["recipe_id"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Skill{ID: 7, Name: "new-skill", RecipeID: 300, FolderID: 30, ProjectID: 12})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	s, err := client.Skills().Create(context.Background(), 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != 7 {
		t.Errorf("ID = %d, want 7", s.ID)
	}
	if s.RecipeID != 300 {
		t.Errorf("RecipeID = %d, want 300", s.RecipeID)
	}
}
