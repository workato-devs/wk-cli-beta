package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

type apiEndpointService struct {
	client *HTTPClient
}

func (s *apiEndpointService) List(ctx context.Context, collectionID *int, opts *PaginationOptions) ([]APIEndpoint, error) {
	params := url.Values{}
	if collectionID != nil {
		params.Set("api_collection_id", strconv.Itoa(*collectionID))
	}
	if opts != nil {
		if opts.Page > 0 {
			params.Set("page", strconv.Itoa(opts.Page))
		}
		if opts.PerPage > 0 {
			params.Set("per_page", strconv.Itoa(opts.PerPage))
		}
	}
	path := "/api_endpoints"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var result []APIEndpoint
	if err := s.client.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *apiEndpointService) Enable(ctx context.Context, id int) error {
	return s.client.do(ctx, "PUT", fmt.Sprintf("/api_endpoints/%d/enable", id), nil, nil)
}

func (s *apiEndpointService) Disable(ctx context.Context, id int) error {
	return s.client.do(ctx, "PUT", fmt.Sprintf("/api_endpoints/%d/disable", id), nil, nil)
}
