package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIEndpointService_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if cid := r.URL.Query().Get("api_collection_id"); cid != "5" {
			t.Errorf("api_collection_id = %q, want 5", cid)
		}
		w.Header().Set("Content-Type", "application/json")
		// Production expects raw array (no wrapper).
		json.NewEncoder(w).Encode([]APIEndpoint{{ID: 1, Name: "ep1", APICollectionID: 5, Active: true, Method: "GET", Path: "/users", RecipeID: 42}})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	cid := 5
	endpoints, err := client.APIEndpoints().List(context.Background(), &cid, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(endpoints) != 1 || !endpoints[0].Active {
		t.Errorf("got %+v, want 1 active endpoint", endpoints)
	}
	ep := endpoints[0]
	if ep.Method != "GET" {
		t.Errorf("Method = %q, want %q", ep.Method, "GET")
	}
	if ep.Path != "/users" {
		t.Errorf("Path = %q, want %q", ep.Path, "/users")
	}
	if ep.RecipeID != 42 {
		t.Errorf("RecipeID = %d, want 42", ep.RecipeID)
	}
}

func TestAPIEndpointService_Enable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/api_endpoints/3/enable" {
			t.Errorf("path = %s, want /api_endpoints/3/enable", r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	err := client.APIEndpoints().Enable(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPIEndpointService_Disable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/api_endpoints/3/disable" {
			t.Errorf("path = %s, want /api_endpoints/3/disable", r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	err := client.APIEndpoints().Disable(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
