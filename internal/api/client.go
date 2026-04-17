package api

import "context"

// RecipeService defines operations on recipes.
type RecipeService interface {
	List(ctx context.Context, opts *RecipeListOptions) ([]Recipe, error)
	Get(ctx context.Context, id int) (*Recipe, error)
	Start(ctx context.Context, id int) error
	Stop(ctx context.Context, id int) error
	Export(ctx context.Context, id int) ([]byte, error)
	Import(ctx context.Context, folderID int, data []byte) (*Recipe, error)
	ListJobs(ctx context.Context, recipeID int, opts *JobListOptions) ([]Job, error)
	Copy(ctx context.Context, recipeID, folderID int) (*Recipe, error)
	Connect(ctx context.Context, recipeID int, adapterName string, connectionID int) error
}

// RecipeListOptions configures recipe list filtering.
type RecipeListOptions struct {
	FolderID *int
	Status   string // "running", "stopped", "all"
	Page     int
	PerPage  int
}

// ConnectionService defines operations on connections.
type ConnectionService interface {
	List(ctx context.Context, opts *ConnectionListOptions) ([]Connection, error)
	Get(ctx context.Context, id int) (*Connection, error)
	Create(ctx context.Context, name, provider string, folderID *int) (*Connection, error)
	Update(ctx context.Context, id int, name string) (*Connection, error)
	Delete(ctx context.Context, id int) error
	Disconnect(ctx context.Context, id int) error
}

// ConnectionListOptions configures connection list filtering.
type ConnectionListOptions struct {
	FolderID *int
	Page     int
	PerPage  int
}

// FolderService defines operations on folders.
type FolderService interface {
	List(ctx context.Context, parentID *int) ([]Folder, error)
	Get(ctx context.Context, id int) (*Folder, error)
	Create(ctx context.Context, name string, parentID *int) (*Folder, error)
	Delete(ctx context.Context, id int) error
}

// PackageService defines operations on RLCM packages (export/import).
type PackageService interface {
	Export(ctx context.Context, folderID int) (int, error)              // returns package ID
	ExportStatus(ctx context.Context, packageID int) (*Package, error)
	Download(ctx context.Context, packageID int) ([]byte, error)
	Import(ctx context.Context, folderID int, data []byte, restartRecipes bool) (int, error) // returns import ID
	ImportStatus(ctx context.Context, importID int) (*Package, error)
}

// TagService defines operations on tags.
type TagService interface {
	List(ctx context.Context, opts *TagListOptions) ([]Tag, error)
	Create(ctx context.Context, title, description, color string) (*Tag, error)
	Update(ctx context.Context, handle string, opts *TagUpdateOptions) (*Tag, error)
	Delete(ctx context.Context, handle string) error
	Assign(ctx context.Context, addTags, removeTags []string, recipeIDs, connectionIDs []int) error
}

// APICollectionService defines operations on API collections.
type APICollectionService interface {
	List(ctx context.Context, opts *PaginationOptions) ([]APICollection, error)
	Create(ctx context.Context, name string, projectID int) (*APICollection, error)
}

// APIEndpointService defines operations on API endpoints.
type APIEndpointService interface {
	List(ctx context.Context, collectionID *int, opts *PaginationOptions) ([]APIEndpoint, error)
	Enable(ctx context.Context, id int) error
	Disable(ctx context.Context, id int) error
}

// WorkspaceService defines operations on workspace management.
type WorkspaceService interface {
	GetCurrentWorkspace(ctx context.Context) (*WorkspaceInfo, error)
	ListMembers(ctx context.Context, email string) ([]WorkspaceUser, error)
	GetAuditLogs(ctx context.Context, opts *AuditLogOptions) ([]AuditLogEntry, error)
}

// ConnectorService defines operations on connectors.
type ConnectorService interface {
	List(ctx context.Context, search string) ([]Connector, error)
}

// Client is the top-level API client providing access to all services.
type Client interface {
	Recipes() RecipeService
	Connections() ConnectionService
	Folders() FolderService
	Packages() PackageService
	Tags() TagService
	APICollections() APICollectionService
	APIEndpoints() APIEndpointService
	Workspace() WorkspaceService
	Connectors() ConnectorService
}
