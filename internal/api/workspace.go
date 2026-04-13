package api

import (
	"context"
	"net/url"
)

type workspaceService struct {
	client *HTTPClient
}

func (s *workspaceService) GetCurrentUser(ctx context.Context) (*WorkspaceUser, error) {
	var user WorkspaceUser
	if err := s.client.do(ctx, "GET", "/users/me", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *workspaceService) ListMembers(ctx context.Context, email string) ([]WorkspaceUser, error) {
	params := url.Values{}
	if email != "" {
		params.Set("email", email)
	}
	path := "/members"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var wrapper struct {
		Data []WorkspaceUser `json:"data"`
	}
	if err := s.client.do(ctx, "GET", path, nil, &wrapper); err != nil {
	var result ResultList[WorkspaceUser]
	if err := s.client.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return wrapper.Data, nil
}

func (s *workspaceService) GetAuditLogs(ctx context.Context, opts *AuditLogOptions) ([]AuditLogEntry, error) {
	params := url.Values{}
	if opts != nil {
		if opts.Since != "" {
			params.Set("from", opts.Since)
		}
		if opts.Until != "" {
			params.Set("to", opts.Until)
		}
		if opts.Action != "" {
			params.Set("event_type", opts.Action)
		}
	}
	path := "/activity_logs"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var wrapper struct {
		Data []AuditLogEntry `json:"data"`
	}
	if err := s.client.do(ctx, "GET", path, nil, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Data, nil
}
