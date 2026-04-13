package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

type folderService struct {
	client *HTTPClient
}

func (s *folderService) List(ctx context.Context, parentID *int) ([]Folder, error) {
	params := url.Values{}
	if parentID != nil {
		params.Set("parent_id", strconv.Itoa(*parentID))
	}
	path := "/folders"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var result []Folder
	if err := s.client.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *folderService) Get(ctx context.Context, id int) (*Folder, error) {
	var folder Folder
	if err := s.client.do(ctx, "GET", fmt.Sprintf("/folders/%d", id), nil, &folder); err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *folderService) Create(ctx context.Context, name string, parentID *int) (*Folder, error) {
	body := map[string]any{"name": name}
	if parentID != nil {
		body["parent_id"] = *parentID
	}
	var folder Folder
	if err := s.client.do(ctx, "POST", "/folders", body, &folder); err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *folderService) Delete(ctx context.Context, id int) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/folders/%d", id), nil, nil)
}
