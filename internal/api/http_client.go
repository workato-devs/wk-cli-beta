package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// HTTPClient implements the Client interface using HTTP.
type HTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	verbose    bool

	recipes        *recipeService
	connections    *connectionService
	folders        *folderService
	packages       *packageService
	tags           *tagService
	apiCollections *apiCollectionService
	apiEndpoints   *apiEndpointService
	skills         *skillService
	workspace      *workspaceService
	connectors     *connectorService
}

// ClientOption configures the HTTPClient.
type ClientOption func(*HTTPClient)

// WithTimeout sets the HTTP timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *HTTPClient) {
		c.httpClient.Timeout = d
	}
}

// WithVerbose enables verbose logging.
func WithVerbose(v bool) ClientOption {
	return func(c *HTTPClient) {
		c.verbose = v
	}
}

// WithHTTPClient sets a custom http.Client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *HTTPClient) {
		c.httpClient = hc
	}
}

// NewHTTPClient creates a new API client.
func NewHTTPClient(baseURL, token string, opts ...ClientOption) *HTTPClient {
	c := &HTTPClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *HTTPClient) Recipes() RecipeService {
	if c.recipes == nil {
		c.recipes = &recipeService{client: c}
	}
	return c.recipes
}

func (c *HTTPClient) Connections() ConnectionService {
	if c.connections == nil {
		c.connections = &connectionService{client: c}
	}
	return c.connections
}

func (c *HTTPClient) Folders() FolderService {
	if c.folders == nil {
		c.folders = &folderService{client: c}
	}
	return c.folders
}

func (c *HTTPClient) Packages() PackageService {
	if c.packages == nil {
		c.packages = &packageService{client: c}
	}
	return c.packages
}

func (c *HTTPClient) Tags() TagService {
	if c.tags == nil {
		c.tags = &tagService{client: c}
	}
	return c.tags
}

func (c *HTTPClient) APICollections() APICollectionService {
	if c.apiCollections == nil {
		c.apiCollections = &apiCollectionService{client: c}
	}
	return c.apiCollections
}

func (c *HTTPClient) APIEndpoints() APIEndpointService {
	if c.apiEndpoints == nil {
		c.apiEndpoints = &apiEndpointService{client: c}
	}
	return c.apiEndpoints
}

func (c *HTTPClient) Skills() SkillService {
	if c.skills == nil {
		c.skills = &skillService{client: c}
	}
	return c.skills
}

func (c *HTTPClient) Workspace() WorkspaceService {
	if c.workspace == nil {
		c.workspace = &workspaceService{client: c}
	}
	return c.workspace
}

func (c *HTTPClient) Connectors() ConnectorService {
	if c.connectors == nil {
		c.connectors = &connectorService{client: c}
	}
	return c.connectors
}

// do executes an HTTP request and decodes the response.
func (c *HTTPClient) do(ctx context.Context, method, path string, body, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", "wk-cli/dev")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	if c.verbose {
		fmt.Fprintf(os.Stderr, "[debug] %s %s%s\n", method, c.baseURL, path)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if c.verbose {
		fmt.Fprintf(os.Stderr, "[debug] HTTP %d %s\n", resp.StatusCode, resp.Status)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
		var errResp struct {
			Message   string `json:"message"`
			Error     string `json:"error"`
			ErrorType string `json:"error_type"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			if errResp.Message != "" {
				apiErr.Message = errResp.Message
			} else if errResp.Error != "" {
				apiErr.Message = errResp.Error
			}
			apiErr.ErrorType = errResp.ErrorType
		}
		return apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// doRaw executes an HTTP request and returns the raw response body.
func (c *HTTPClient) doRaw(ctx context.Context, method, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", "wk-cli/dev")

	if c.verbose {
		fmt.Fprintf(os.Stderr, "[debug] %s %s%s\n", method, c.baseURL, path)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if c.verbose {
		fmt.Fprintf(os.Stderr, "[debug] HTTP %d %s\n", resp.StatusCode, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(data),
		}
	}
	return data, nil
}
