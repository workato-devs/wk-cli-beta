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

// RecipeVersion represents a single entry in a recipe's version history
// (GET /recipes/:id/versions). Comment is a pointer because the API may
// return null for versions that were never commented; *string preserves
// the distinction between "no comment" and "empty comment".
type RecipeVersion struct {
	ID          int       `json:"id"`
	VersionNo   int       `json:"version_no"`
	Comment     *string   `json:"comment,omitempty"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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

// Folder represents a Workato folder. IsProject distinguishes top-level
// projects from plain folders — the Workato workspace treats them as
// the same resource shape on list, but delete routes differently:
// projects require DELETE /projects/{project_id}; plain folders use
// DELETE /folders/{id}.
//
// ProjectID is populated by the list response when IsProject is true
// and is the identifier that DELETE /projects/... requires — distinct
// from ID (the folder id) even when the folder IS a project.
type Folder struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ParentID  *int   `json:"parent_id,omitempty"`
	IsProject bool   `json:"is_project,omitempty"`
	ProjectID int    `json:"project_id,omitempty"`
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

// Job represents a recipe job execution. Job IDs are strings
// (e.g. "j-AJMfQh8c-hsCXcs"); recipe IDs are integers.
type Job struct {
	ID              string     `json:"id"`
	RecipeID        int        `json:"recipe_id"`
	Status          string     `json:"status"` // "succeeded", "failed", "pending"
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Title           string     `json:"title,omitempty"`
	IsError         bool       `json:"is_error"`
	Error           *string    `json:"error,omitempty"`
	IsPollError     bool       `json:"is_poll_error"`
	CallingRecipeID *int       `json:"calling_recipe_id,omitempty"`
	CallingJobID    *string    `json:"calling_job_id,omitempty"`
	RootRecipeID    *int       `json:"root_recipe_id,omitempty"`
	RootJobID       *string    `json:"root_job_id,omitempty"`
	MasterJobID     *string    `json:"master_job_id,omitempty"`
}

// JobDetail is the single-job response from GET /recipes/{id}/jobs/{job_id}.
type JobDetail struct {
	Job
	Handle           string    `json:"handle,omitempty"`
	IsRepeat         bool      `json:"is_repeat"`
	IsTest           bool      `json:"is_test"`
	IsTestCaseJob    bool      `json:"is_test_case_job"`
	MasterJobHandle  string    `json:"master_job_handle,omitempty"`
	CallingJobHandle string    `json:"calling_job_handle,omitempty"`
	Lines            []JobLine `json:"lines,omitempty"`
}

// JobLine represents a single step in a job execution trace.
type JobLine struct {
	RecipeLineNumber int       `json:"recipe_line_number"`
	AdapterName      string    `json:"adapter_name"`
	AdapterOperation string    `json:"adapter_operation"`
	LineStat         *LineStat `json:"line_stat,omitempty"`
}

// LineStat holds timing data for a job step.
type LineStat struct {
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

// APIEndpoint represents a Workato API endpoint. The create response returns
// "flow_id" where the list response returns "recipe_id"; FlowID captures the
// former so both paths decode cleanly.
type APIEndpoint struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	APICollectionID int    `json:"api_collection_id"`
	Active          bool   `json:"active"`
	Method          string `json:"method,omitempty"`
	Path            string `json:"path,omitempty"`
	RecipeID        int    `json:"recipe_id,omitempty"`
	FlowID          int    `json:"flow_id,omitempty"`
}

// Skill represents a Workato agentic skill. The API returns string IDs
// (e.g. "skl-Aa6zhmTh-4ac8TH-AB") and uses "provider_id" for the recipe
// association; RecipeID is backfilled from ProviderID for display convenience.
type Skill struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	RecipeID           int    `json:"recipe_id,omitempty"`
	ProviderID         int    `json:"provider_id"`
	ProviderType       string `json:"provider_type,omitempty"`
	FolderID           int    `json:"folder_id"`
	ProjectID          int    `json:"project_id"`
	Running            bool   `json:"running"`
	GeniesCount        int    `json:"genies_count"`
	TriggerDescription string `json:"trigger_description,omitempty"`
	Applications       []any  `json:"applications,omitempty"`
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

// WorkspaceInfo is the shape returned by GET /users/me. Despite the endpoint
// path, the response describes the workspace the token authenticates against:
// id and name are the workspace's. Email is the authenticated account's email.
type WorkspaceInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// WorkspaceUser represents a Workato workspace member (from GET /members).
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
