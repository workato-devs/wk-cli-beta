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
		w.Write([]byte(`{"data":[{"id":"skl-001","name":"my-skill","description":"test skill","provider_id":100,"provider_type":"Recipe","folder_id":10,"project_id":5,"running":true,"genies_count":2}]}`))
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
	if s.ID != "skl-001" {
		t.Errorf("ID = %q, want skl-001", s.ID)
	}
	if s.Description != "test skill" {
		t.Errorf("Description = %q, want %q", s.Description, "test skill")
	}
	if s.ProviderID != 100 {
		t.Errorf("ProviderID = %d, want 100", s.ProviderID)
	}
	if s.RecipeID != 100 {
		t.Errorf("RecipeID = %d, want 100 (backfilled from ProviderID)", s.RecipeID)
	}
	if s.FolderID != 10 {
		t.Errorf("FolderID = %d, want 10", s.FolderID)
	}
	if s.ProjectID != 5 {
		t.Errorf("ProjectID = %d, want 5", s.ProjectID)
	}
	if !s.Running {
		t.Errorf("Running = false, want true")
	}
	if s.GeniesCount != 2 {
		t.Errorf("GeniesCount = %d, want 2", s.GeniesCount)
	}
}

func TestSkillService_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/agentic/skills/skl-042" {
			t.Errorf("path = %s, want /agentic/skills/skl-042", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"id":"skl-042","name":"lookup-skill","provider_id":200,"folder_id":20,"project_id":8,"running":false,"genies_count":0}}`))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	s, err := client.Skills().Get(context.Background(), "skl-042")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "skl-042" {
		t.Errorf("ID = %q, want skl-042", s.ID)
	}
	if s.Name != "lookup-skill" {
		t.Errorf("Name = %q, want %q", s.Name, "lookup-skill")
	}
	if s.RecipeID != 200 {
		t.Errorf("RecipeID = %d, want 200 (backfilled from ProviderID)", s.RecipeID)
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
		w.Write([]byte(`{"data":{"id":"skl-007","name":"new-skill","provider_id":300,"provider_type":"Recipe","folder_id":30,"project_id":12,"running":true,"genies_count":0}}`))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	s, err := client.Skills().Create(context.Background(), 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "skl-007" {
		t.Errorf("ID = %q, want skl-007", s.ID)
	}
	if s.RecipeID != 300 {
		t.Errorf("RecipeID = %d, want 300 (backfilled from ProviderID)", s.RecipeID)
	}
	if s.ProviderType != "Recipe" {
		t.Errorf("ProviderType = %q, want Recipe", s.ProviderType)
	}
}
