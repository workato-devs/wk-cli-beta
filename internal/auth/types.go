package auth

import "time"

// Region represents a Workato data center region.
type Region string

const (
	RegionUS    Region = "us"
	RegionEU    Region = "eu"
	RegionJP    Region = "jp"
	RegionAU    Region = "au"
	RegionSG    Region = "sg"
	RegionIL    Region = "il"
	RegionCN    Region = "cn"
	RegionTrial Region = "trial"
)

// ValidRegions returns all supported regions.
func ValidRegions() []Region {
	return []Region{RegionUS, RegionEU, RegionJP, RegionAU, RegionSG, RegionIL, RegionCN, RegionTrial}
}

// IsValid checks if a region string is a supported region.
func (r Region) IsValid() bool {
	for _, v := range ValidRegions() {
		if v == r {
			return true
		}
	}
	return false
}

// StoreType identifies the credential storage backend.
type StoreType string

const (
	StoreKeychain StoreType = "keychain"
	StoreFile     StoreType = "file"
	StoreVault    StoreType = "vault"
)

// Credential holds an API token and its metadata.
type Credential struct {
	Token     string     `json:"token"`
	Region    Region     `json:"region"`
	StoreType StoreType  `json:"store_type"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Profile represents a named authentication profile targeting a specific
// workspace, environment, and region combination.
//
// Workspace, WorkspaceID, and Email are populated from GET /users/me at
// login time (see ADR-006). Environment is user-provided.
type Profile struct {
	Name        string     `json:"name"`
	Workspace   string     `json:"workspace"`
	WorkspaceID int        `json:"workspace_id,omitempty"`
	Environment string     `json:"environment"`
	Email       string     `json:"email,omitempty"`
	Region      Region     `json:"region"`
	StoreType   StoreType  `json:"store_type"`
	BaseURL     string     `json:"base_url"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}
