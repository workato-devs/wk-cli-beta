package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

type skillService struct {
	client *HTTPClient
}

// skillListResponse wraps the paginated response from GET /agentic/skills.
type skillListResponse struct {
	Data []Skill `json:"data"`
}

func (s *skillService) List(ctx context.Context, opts *PaginationOptions) ([]Skill, error) {
	params := url.Values{}
	if opts != nil {
		if opts.Page > 0 {
			params.Set("page", strconv.Itoa(opts.Page))
		}
		if opts.PerPage > 0 {
			params.Set("per_page", strconv.Itoa(opts.PerPage))
		}
	}
	path := "/agentic/skills"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var result skillListResponse
	if err := s.client.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (s *skillService) Get(ctx context.Context, id int) (*Skill, error) {
	var skill Skill
	if err := s.client.do(ctx, "GET", fmt.Sprintf("/agentic/skills/%d", id), nil, &skill); err != nil {
		return nil, err
	}
	return &skill, nil
}

func (s *skillService) Create(ctx context.Context, recipeID int) (*Skill, error) {
	body := map[string]any{
		"recipe_id": recipeID,
	}
	var skill Skill
	if err := s.client.do(ctx, "POST", "/agentic/skills", body, &skill); err != nil {
		return nil, err
	}
	return &skill, nil
}
