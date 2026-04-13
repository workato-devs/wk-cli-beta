package api

import "time"

// Recipe represents a Workato recipe.
type Recipe struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	FolderID    int       `json:"folder_id"`
	Running     bool      `json:"running"`
	Active      bool      `json:"active"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Code        any       `json:"code,omitempty"`
	Config      any       `json:"config,omitempty"`
}

// Connection represents a Workato connection.
type Connection struct {
	ID                  int       `json:"id"`
	Name                string    `json:"name"`
	Application         string    `json:"application"`
	FolderID            int       `json:"folder_id"`
	AuthorizationStatus *string   `json:"authorization_status"`
	AuthorizationError  *string   `json:"authorization_error"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Folder represents a Workato folder.
type Folder struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ParentID *int   `json:"parent_id,omitempty"`
}

// Package represents an RLCM export/import package.
type Package struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	ErrorParts []any     `json:"error_parts,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ExportManifest represents a Workato RLCM export manifest.
// Creating a manifest is required before triggering a package export.
type ExportManifest struct {
	ID       int    `json:"id"`
	Name     string `json:"name,omitempty"`
	Status   string `json:"status,omitempty"`
	FolderID int    `json:"folder_id,omitempty"`
}

// PackageContent describes a single asset within an RLCM package.
type PackageContent struct {
	AbsolutePath string `json:"absolute_path"`
	ZipName      string `json:"zip_name"`
	Folder       string `json:"folder"`
	Type         string `json:"type"` // "recipe", "connection", etc.
}

// Job represents a recipe job execution.
type Job struct {
	ID          int        `json:"id"`
	RecipeID    int        `json:"recipe_id"`
	Status      string     `json:"status"` // "succeeded", "failed", "pending"
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ListResult is a generic wrapper for paginated API responses
// that return {"items":[...]}, e.g. recipes and jobs.
type ListResult[T any] struct {
	Items []T `json:"items"`
}

// Tag represents a Workato tag.
type Tag struct {
	Handle      string `json:"handle"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// TagListOptions configures tag list filtering.
type TagListOptions struct {
	Search  string
	Page    int
	PerPage int
}

// TagUpdateOptions configures tag updates.
type TagUpdateOptions struct {
	Title       *string
	Description *string
	Color       *string
}

// APICollection represents a Workato API collection.
type APICollection struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Handle      string `json:"handle,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	UsePrefix   bool   `json:"use_prefix,omitempty"`
	ProjectID   int    `json:"project_id"`
}

// APIEndpoint represents a Workato API endpoint.
type APIEndpoint struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	APICollectionID int    `json:"api_collection_id"`
	Active          bool   `json:"active"`
	Method          string `json:"method,omitempty"`
	Path            string `json:"path,omitempty"`
	RecipeID        int    `json:"recipe_id,omitempty"`
}

// PaginationOptions provides generic pagination parameters.
type PaginationOptions struct {
	Page    int
	PerPage int
}

// MCPServerInfo represents the result of an MCP initialize handshake.
type MCPServerInfo struct {
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	ProtocolVersion string         `json:"protocol_version"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
}

// MCPTool represents a tool exposed by an MCP server.
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

// WorkspaceUser represents a Workato workspace member.
type WorkspaceUser struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// AuditLogEntry represents a Workato audit log entry.
type AuditLogEntry struct {
	ID        int    `json:"id"`
	EventType string `json:"event_type"`
	Timestamp string `json:"timestamp"`
	User      struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"user"`
	Details any `json:"details,omitempty"`
}

// AuditLogOptions configures audit log filtering.
type AuditLogOptions struct {
	Since  string
	Until  string
	Action string
}

// JobListOptions configures job list filtering.
type JobListOptions struct {
	Status string
	Limit  int
}

// Connector represents a Workato connector (integration).
type Connector struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}
