package auth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/workato-devs/wk-cli-beta/internal/config"
	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

// ProfilesEnvFile is the fixed filename for the file-based credential store.
const ProfilesEnvFile = "profiles.env"

// FileStore reads profiles and credentials from a project-local profiles.env.
// The CLI reads this file but never writes to it (ADR-006 Sub-decision 3).
// The file is developer- or pipeline-authored.
//
// The file lives at <projectRoot>/.wk/profiles.env — alongside wk.toml inside
// the tool-managed directory. Placing it there means it's automatically
// hidden by .wk/.gitignore (ADR-005 Decision 8), which matters because
// profiles.env holds API tokens and must never be committed.
type FileStore struct {
	Path string // absolute path to profiles.env
}

// NewFileStore returns a FileStore anchored at <projectRoot>/.wk/profiles.env.
func NewFileStore(projectRoot string) *FileStore {
	return &FileStore{Path: filepath.Join(projectRoot, config.ProjectDir, ProfilesEnvFile)}
}

// Exists reports whether the backing file is present.
func (f *FileStore) Exists() bool {
	if f == nil || f.Path == "" {
		return false
	}
	_, err := os.Stat(f.Path)
	return err == nil
}

// load opens the file and parses all records.
func (f *FileStore) load() ([]fileRecord, error) {
	file, err := os.Open(f.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", f.Path, err)
	}
	defer file.Close()
	return parseProfilesEnv(file)
}

// GetProfile returns the profile metadata for name.
func (f *FileStore) GetProfile(name string) (*Profile, error) {
	records, err := f.load()
	if err != nil {
		return nil, err
	}
	for _, rec := range records {
		if rec.name == name {
			return rec.toProfile(), nil
		}
	}
	return nil, wkerrors.ErrProfileNotFound
}

// Get implements CredentialStore. Returns the credential for name.
func (f *FileStore) Get(_ context.Context, name string) (*Credential, error) {
	records, err := f.load()
	if err != nil {
		return nil, err
	}
	for _, rec := range records {
		if rec.name == name {
			return rec.toCredential(), nil
		}
	}
	return nil, wkerrors.ErrCredentialNotFound
}

// Set is not supported. The CLI does not write to profiles.env.
func (f *FileStore) Set(_ context.Context, _ string, _ *Credential) error {
	return wkerrors.ErrProfilesEnvReadOnly
}

// Delete is not supported. The CLI does not write to profiles.env.
func (f *FileStore) Delete(_ context.Context, _ string) error {
	return wkerrors.ErrProfilesEnvReadOnly
}

// List returns all profile names in file order.
func (f *FileStore) List(_ context.Context) ([]string, error) {
	records, err := f.load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(records))
	for _, rec := range records {
		names = append(names, rec.name)
	}
	return names, nil
}

// ListProfiles returns all profiles in file order.
func (f *FileStore) ListProfiles() ([]*Profile, error) {
	records, err := f.load()
	if err != nil {
		return nil, err
	}
	profiles := make([]*Profile, 0, len(records))
	for _, rec := range records {
		profiles = append(profiles, rec.toProfile())
	}
	return profiles, nil
}

// fileRecord is a parsed profiles.env entry. Fields are the raw strings from
// the file; toProfile/toCredential do the conversion.
type fileRecord struct {
	name        string
	workspace   string
	environment string
	region      string
	baseURL     string
	token       string
	// startLine is the 1-based line number of the NAME= line, used for
	// error messages from consumers.
	startLine int //nolint:unused // reserved for future error messaging
}

func (r fileRecord) toProfile() *Profile {
	region := Region(r.region)
	baseURL := r.baseURL
	if baseURL == "" {
		baseURL = config.BaseURL(string(region))
	}
	return &Profile{
		Name:        r.name,
		Workspace:   r.workspace,
		Environment: r.environment,
		Region:      region,
		StoreType:   StoreFile,
		BaseURL:     baseURL,
		CreatedAt:   time.Time{}, // unknown for file-store profiles
	}
}

func (r fileRecord) toCredential() *Credential {
	return &Credential{
		Token:     r.token,
		Region:    Region(r.region),
		StoreType: StoreFile,
		CreatedAt: time.Now(),
	}
}

// parseProfilesEnv parses a profiles.env reader into a slice of records.
// See ADR-006 Sub-decision 3 for the format contract.
//
// Strict on structure: a field before any NAME= is an error; duplicate keys
// within one record is an error; missing required fields (NAME, TOKEN,
// REGION) is an error. Permissive on unknown keys: they produce a warning
// to stderr but parsing continues.
func parseProfilesEnv(r io.Reader) ([]fileRecord, error) {
	scanner := bufio.NewScanner(r)
	var records []fileRecord
	var current *fileRecord
	var seenKeys map[string]bool
	lineNo := 0

	flush := func() error {
		if current == nil {
			return nil
		}
		if current.name == "" {
			return fmt.Errorf("profiles.env: record near line %d missing NAME=", current.startLine)
		}
		if current.token == "" {
			return fmt.Errorf("profiles.env: record %q (line %d) missing required field TOKEN", current.name, current.startLine)
		}
		if current.region == "" {
			return fmt.Errorf("profiles.env: record %q (line %d) missing required field REGION", current.name, current.startLine)
		}
		records = append(records, *current)
		return nil
	}

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return nil, fmt.Errorf("profiles.env line %d: expected KEY=VALUE, got %q", lineNo, line)
		}
		key := strings.TrimSpace(line[:eq])
		val := unquote(strings.TrimSpace(line[eq+1:]))

		if key == "NAME" {
			if err := flush(); err != nil {
				return nil, err
			}
			current = &fileRecord{name: val, startLine: lineNo}
			seenKeys = map[string]bool{"NAME": true}
			continue
		}

		if current == nil {
			return nil, fmt.Errorf("profiles.env line %d: field %q appears before any NAME=", lineNo, key)
		}
		if seenKeys[key] {
			return nil, fmt.Errorf("profiles.env line %d: duplicate %s= in record %q", lineNo, key, current.name)
		}
		seenKeys[key] = true

		switch key {
		case "WORKSPACE":
			current.workspace = val
		case "ENVIRONMENT":
			current.environment = val
		case "REGION":
			current.region = val
		case "BASE_URL":
			current.baseURL = val
		case "TOKEN":
			current.token = val
		case "STORE_TYPE":
			if val != "" && val != string(StoreFile) {
				return nil, fmt.Errorf("profiles.env line %d: STORE_TYPE=%q invalid for file store (expected %q or unset)", lineNo, val, StoreFile)
			}
		default:
			fmt.Fprintf(os.Stderr, "warning: profiles.env line %d: unknown key %q (ignored)\n", lineNo, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return records, nil
}

// unquote strips matching outer double quotes. It does not process escapes.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
