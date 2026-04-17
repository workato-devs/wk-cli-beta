package auth

import "context"

// CredentialStore is the interface for credential storage backends.
// Current implementations: KeyringStore (OS keychain) and FileStore
// (project-level profiles.env). Routing to the right backend is
// handled by the commands layer per ADR-006 Sub-decision 6.
type CredentialStore interface {
	// Get retrieves the credential for a named profile.
	Get(ctx context.Context, profileName string) (*Credential, error)

	// Set stores a credential for a named profile.
	Set(ctx context.Context, profileName string, cred *Credential) error

	// Delete removes the credential for a named profile.
	Delete(ctx context.Context, profileName string) error

	// List returns all profile names with stored credentials.
	List(ctx context.Context) ([]string, error)
}
