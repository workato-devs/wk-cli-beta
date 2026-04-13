package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

type tagService struct {
	client *HTTPClient
}

func (s *tagService) List(ctx context.Context, opts *TagListOptions) ([]Tag, error) {
	params := url.Values{}
	if opts != nil {
		if opts.Search != "" {
			params.Set("q", opts.Search)
		}
		if opts.Page > 0 {
			params.Set("page", strconv.Itoa(opts.Page))
		}
		if opts.PerPage > 0 {
			params.Set("per_page", strconv.Itoa(opts.PerPage))
		}
	}
	path := "/tags"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var wrapper struct {
		Data struct {
			Tags []Tag `json:"tags"`
		} `json:"data"`
	}
	if err := s.client.do(ctx, "GET", path, nil, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Data.Tags, nil
}

func (s *tagService) Create(ctx context.Context, title, description, color string) (*Tag, error) {
	body := map[string]any{"title": title}
	if description != "" {
		body["description"] = description
	}
	if color != "" {
		body["color"] = color
	}
	var tag Tag
	if err := s.client.do(ctx, "POST", "/tags", body, &tag); err != nil {
		return nil, err
	}
	return &tag, nil
}

func (s *tagService) Update(ctx context.Context, handle string, opts *TagUpdateOptions) (*Tag, error) {
	body := map[string]any{}
	if opts != nil {
		if opts.Title != nil {
			body["title"] = *opts.Title
		}
		if opts.Description != nil {
			body["description"] = *opts.Description
		}
		if opts.Color != nil {
			body["color"] = *opts.Color
		}
	}
	var tag Tag
	if err := s.client.do(ctx, "PUT", fmt.Sprintf("/tags/%s", handle), body, &tag); err != nil {
		return nil, err
	}
	return &tag, nil
}

func (s *tagService) Delete(ctx context.Context, handle string) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/tags/%s", handle), nil, nil)
}

func (s *tagService) Assign(ctx context.Context, addTags, removeTags []string, recipeIDs, connectionIDs []int) error {
	body := map[string]any{}
	if len(addTags) > 0 {
		body["add_tags"] = addTags
	}
	if len(removeTags) > 0 {
		body["remove_tags"] = removeTags
	}
	if len(recipeIDs) > 0 {
		body["recipe_ids"] = recipeIDs
	}
	if len(connectionIDs) > 0 {
		body["connection_ids"] = connectionIDs
	}
	return s.client.do(ctx, "POST", "/tags_assignments", body, nil)
}
