package errors

import (
	"errors"
	"fmt"
)

// Exit codes returned by the CLI process. Agents and scripts can switch
// on these to distinguish error categories without parsing stderr text.
const (
	ExitOK         = 0
	ExitGeneral    = 1
	ExitValidation = 2
	ExitAuth       = 3
	ExitNoProject  = 4
	ExitAPI        = 5
	ExitPlugin     = 6
)

// Sentinel errors for the wk CLI.
var (
	// Auth errors
	ErrNoActiveProfile    = errors.New("no active auth profile configured")
	ErrProfileNotFound    = errors.New("auth profile not found")
	ErrCredentialNotFound = errors.New("credential not found in store")
	ErrTokenExpired       = errors.New("auth token has expired")
	ErrProfileMismatch    = errors.New("active profile does not match project's configured profile")
	ErrDuplicateTarget    = errors.New("a profile already targets this workspace/environment/region combination")
	ErrProfilesEnvReadOnly = errors.New("profiles.env is read-only by design; edit the file directly")

	// Project errors
	ErrNotInProject    = errors.New("not in a wk project directory (no wk.toml found)")
	ErrProjectExists   = errors.New("wk.toml already exists in this directory")
	ErrNestedProject   = errors.New("cannot create project inside an existing wk project")

	// Sync errors
	ErrSyncConflict   = errors.New("sync conflict: local and remote changes detected")
	ErrNoSyncEntries  = errors.New("no [[sync]] entries configured in wk.toml")
	ErrMetaCorrupted  = errors.New("sidecar .meta.json is corrupted or missing")

	// Plugin errors
	ErrPluginNotFound = errors.New("plugin not found")
	ErrPluginTimeout  = errors.New("plugin process timed out")
	ErrPluginProtocol = errors.New("plugin JSON-RPC protocol error")

	// API errors
	ErrAPIUnauthorized = errors.New("API request unauthorized (401)")
	ErrAPIForbidden    = errors.New("API request forbidden (403)")
	ErrAPINotFound     = errors.New("API resource not found (404)")
	ErrAPIRateLimit    = errors.New("API rate limit exceeded (429)")
	ErrAPIServer       = errors.New("API server error (5xx)")
)

type coded struct {
	msg      string
	sentinel error
}

func (e *coded) Error() string  { return e.msg }
func (e *coded) Unwrap() error  { return e.sentinel }

func WithSentinel(sentinel error, format string, args ...any) error {
	return &coded{msg: fmt.Sprintf(format, args...), sentinel: sentinel}
}
