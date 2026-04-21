package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

type recipeService struct {
	client *HTTPClient
}

func (s *recipeService) List(ctx context.Context, opts *RecipeListOptions) ([]Recipe, error) {
	params := url.Values{}
	if opts != nil {
		if opts.FolderID != nil {
			params.Set("folder_id", strconv.Itoa(*opts.FolderID))
		}
		if opts.Status == "running" {
			params.Set("active", "true")
		} else if opts.Status == "stopped" {
			params.Set("active", "false")
		}
		if opts.Page > 0 {
			params.Set("page", strconv.Itoa(opts.Page))
		}
		if opts.PerPage > 0 {
			params.Set("per_page", strconv.Itoa(opts.PerPage))
		}
	}
	path := "/recipes"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var result ListResult[Recipe]
	if err := s.client.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (s *recipeService) Get(ctx context.Context, id int) (*Recipe, error) {
	var recipe Recipe
	if err := s.client.do(ctx, "GET", fmt.Sprintf("/recipes/%d", id), nil, &recipe); err != nil {
		return nil, err
	}
	return &recipe, nil
}

func (s *recipeService) Start(ctx context.Context, id int) error {
	return s.client.do(ctx, "PUT", fmt.Sprintf("/recipes/%d/start", id), nil, nil)
}

func (s *recipeService) Stop(ctx context.Context, id int) error {
	return s.client.do(ctx, "PUT", fmt.Sprintf("/recipes/%d/stop", id), nil, nil)
}

func (s *recipeService) Export(ctx context.Context, id int) ([]byte, error) {
	return s.client.doRaw(ctx, "GET", fmt.Sprintf("/recipes/%d", id))
}

func (s *recipeService) Import(ctx context.Context, folderID int, data []byte) (*Recipe, error) {
	body, err := decodeRecipeBody(data)
	if err != nil {
		return nil, err
	}
	body["folder_id"] = strconv.Itoa(folderID)

	var result struct {
		Success bool `json:"success"`
		ID      int  `json:"id"`
	}
	if err := s.client.do(ctx, "POST", "/recipes", body, &result); err != nil {
		return nil, err
	}
	return s.Get(ctx, result.ID)
}

// Update replaces an existing recipe's code/config via PUT /recipes/{id}.
// Shares the same stringification rules as Import — the Workato API expects
// "code" and "config" as JSON-encoded strings even though exports return
// them as objects.
func (s *recipeService) Update(ctx context.Context, id int, data []byte) error {
	body, err := decodeRecipeBody(data)
	if err != nil {
		return err
	}
	// folder_id is meaningful only on create; drop it so callers can reuse
	// their export JSON without accidentally moving the recipe.
	delete(body, "folder_id")
	return s.client.do(ctx, "PUT", fmt.Sprintf("/recipes/%d", id), body, nil)
}

// Delete removes a recipe via DELETE /recipes/{id}. The API returns 204
// on success; s.client.do already treats 2xx as OK.
func (s *recipeService) Delete(ctx context.Context, id int) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/recipes/%d", id), nil, nil)
}

// decodeRecipeBody unmarshals a recipe export JSON and stringifies the
// nested "code" / "config" fields so the body round-trips cleanly back
// through the Workato API's create/update endpoints.
func decodeRecipeBody(data []byte) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("invalid recipe JSON: %w", err)
	}
	for _, key := range []string{"code", "config"} {
		v, ok := body[key]
		if !ok {
			continue
		}
		if _, isString := v.(string); isString {
			continue
		}
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("encoding %s: %w", key, err)
		}
		body[key] = string(encoded)
	}
	return body, nil
}

func (s *recipeService) ListJobs(ctx context.Context, recipeID int, opts *JobListOptions) ([]Job, error) {
	params := url.Values{}
	if opts != nil {
		if opts.Status != "" && opts.Status != "all" {
			params.Set("status", opts.Status)
		}
		if opts.Limit > 0 {
			params.Set("per_page", strconv.Itoa(opts.Limit))
		}
	}
	path := fmt.Sprintf("/recipes/%d/jobs", recipeID)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var result ListResult[Job]
	if err := s.client.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (s *recipeService) Copy(ctx context.Context, recipeID, folderID int) (*Recipe, error) {
	body := map[string]any{"folder_id": folderID}
	var recipe Recipe
	if err := s.client.do(ctx, "POST", fmt.Sprintf("/recipes/%d/copy", recipeID), body, &recipe); err != nil {
		return nil, err
	}
	return &recipe, nil
}

// ListVersions returns the version history for a recipe. Pagination matches
// the Workato contract: default page size 100, max 100. page/perPage values
// <= 0 omit the query params so the server applies its defaults.
//
// Note the response wrapper is `{"data": [...]}`, not `{"items": [...]}`
// like most list endpoints — the generic ListResult[T] would not decode
// it, so the wrapper is inline.
func (s *recipeService) ListVersions(ctx context.Context, recipeID, page, perPage int) ([]RecipeVersion, error) {
	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if perPage > 0 {
		params.Set("per_page", strconv.Itoa(perPage))
	}
	path := fmt.Sprintf("/recipes/%d/versions", recipeID)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var result struct {
		Data []RecipeVersion `json:"data"`
	}
	if err := s.client.do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetVersion returns a single version's metadata.
func (s *recipeService) GetVersion(ctx context.Context, recipeID, versionID int) (*RecipeVersion, error) {
	var v RecipeVersion
	if err := s.client.do(ctx, "GET", fmt.Sprintf("/recipes/%d/versions/%d", recipeID, versionID), nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// UpdateVersionComment sets a comment on a specific version via PATCH.
// The API caps the comment at 255 characters; the check is enforced here
// so the caller sees a clear local error instead of an API 4xx.
func (s *recipeService) UpdateVersionComment(ctx context.Context, recipeID, versionID int, comment string) (*RecipeVersion, error) {
	if len(comment) > 255 {
		return nil, fmt.Errorf("comment exceeds 255-character limit (got %d)", len(comment))
	}
	body := map[string]any{"comment": comment}
	var v RecipeVersion
	if err := s.client.do(ctx, "PATCH", fmt.Sprintf("/recipes/%d/versions/%d", recipeID, versionID), body, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *recipeService) Connect(ctx context.Context, recipeID int, adapterName string, connectionID int) error {
	body := map[string]any{
		"adapter_name":  adapterName,
		"connection_id": connectionID,
	}
	return s.client.do(ctx, "PUT", fmt.Sprintf("/recipes/%d/connect", recipeID), body, nil)
}

