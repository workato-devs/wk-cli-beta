package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

type connectionService struct {
	client *HTTPClient
}

func (s *connectionService) List(ctx context.Context, opts *ConnectionListOptions) ([]Connection, error) {
	params := url.Values{}
	if opts != nil {
		if opts.FolderID != nil {
			params.Set("folder_id", strconv.Itoa(*opts.FolderID))
		}
		if opts.Page > 0 {
			params.Set("page", strconv.Itoa(opts.Page))
		}
		if opts.PerPage > 0 {
			params.Set("per_page", strconv.Itoa(opts.PerPage))
		}
	}
	path := "/connections"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var result []Connection
	if err := s.client.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *connectionService) Get(ctx context.Context, id int) (*Connection, error) {
	// The Workato API has no single-connection GET endpoint.
	// Filter from the list instead.
	conns, err := s.List(ctx, nil)
	if err != nil {
		return nil, err
	}
	for i := range conns {
		if conns[i].ID == id {
			return &conns[i], nil
		}
	}
	return nil, &APIError{StatusCode: 404, Message: fmt.Sprintf("connection %d not found", id)}
}

func (s *connectionService) Create(ctx context.Context, name, provider string, folderID *int) (*Connection, error) {
	body := map[string]any{
		"name":     name,
		"provider": provider,
	}
	if folderID != nil {
		body["folder_id"] = *folderID
	}
	var conn Connection
	if err := s.client.do(ctx, "POST", "/connections", body, &conn); err != nil {
		return nil, err
	}
	return &conn, nil
}

func (s *connectionService) Update(ctx context.Context, id int, name string) (*Connection, error) {
	body := map[string]any{"name": name}
	var conn Connection
	if err := s.client.do(ctx, "PUT", fmt.Sprintf("/connections/%d", id), body, &conn); err != nil {
		return nil, err
	}
	return &conn, nil
}

func (s *connectionService) Delete(ctx context.Context, id int) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/connections/%d", id), nil, nil)
}

func (s *connectionService) Disconnect(ctx context.Context, id int) error {
	return s.client.do(ctx, "POST", fmt.Sprintf("/connections/%d/disconnect", id), nil, nil)
}

