package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectorService_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/integrations" {
			t.Errorf("path = %s, want /integrations", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items": []Connector{{Name: "salesforce", Title: "Salesforce"}},
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	connectors, err := client.Connectors().List(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(connectors) != 1 || connectors[0].Name != "salesforce" {
		t.Errorf("got %+v, want 1 connector named salesforce", connectors)
	}
}

func TestConnectorService_ListWithSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("applications") != "slack" {
			t.Errorf("applications = %q, want slack", r.URL.Query().Get("applications"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items": []Connector{{Name: "slack", Title: "Slack"}},
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	connectors, err := client.Connectors().List(context.Background(), "slack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(connectors) != 1 {
		t.Errorf("got %d connectors, want 1", len(connectors))
	}
}
