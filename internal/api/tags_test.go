package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTagService_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/tags" {
			t.Errorf("path = %s, want /tags", r.URL.Path)
		}
		if q := r.URL.Query().Get("q"); q != "test" {
			t.Errorf("q = %q, want %q", q, "test")
		}
		w.Header().Set("Content-Type", "application/json")
		// Production expects {"data": {"tags": [...]}}.
		json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"tags": []Tag{{Handle: "h1", Title: "Tag 1"}},
				},
			})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	tags, err := client.Tags().List(context.Background(), &TagListOptions{Search: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 1 || tags[0].Handle != "h1" {
		t.Errorf("got %+v, want 1 tag with handle h1", tags)
	}
}

func TestTagService_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["title"] != "My Tag" {
			t.Errorf("title = %v, want My Tag", body["title"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Tag{Handle: "my-tag", Title: "My Tag"})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	tag, err := client.Tags().Create(context.Background(), "My Tag", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag.Handle != "my-tag" {
		t.Errorf("handle = %q, want %q", tag.Handle, "my-tag")
	}
}

func TestTagService_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/tags/my-tag" {
			t.Errorf("path = %s, want /tags/my-tag", r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	err := client.Tags().Delete(context.Background(), "my-tag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTagService_Assign(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/tags_assignments" {
			t.Errorf("path = %s, want /tags_assignments", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		addTags, ok := body["add_tags"].([]any)
		if !ok || len(addTags) != 1 {
			t.Errorf("add_tags = %v, want [my-tag]", body["add_tags"])
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	err := client.Tags().Assign(context.Background(), []string{"my-tag"}, nil, []int{1}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTagService_Update(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/tags/my-tag" {
			t.Errorf("path = %s, want /tags/my-tag", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Tag{Handle: "my-tag", Title: "Updated"})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "test-token")
	title := "Updated"
	tag, err := client.Tags().Update(context.Background(), "my-tag", &TagUpdateOptions{Title: &title})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag.Title != "Updated" {
		t.Errorf("title = %q, want %q", tag.Title, "Updated")
	}
}
