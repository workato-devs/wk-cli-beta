package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProfilesEnv(t *testing.T, content string) *FileStore {
	t.Helper()
	dir := t.TempDir()
	fs := NewFileStore(dir)
	if err := os.MkdirAll(filepath.Dir(fs.Path), 0755); err != nil {
		t.Fatalf("creating .wk/: %v", err)
	}
	if err := os.WriteFile(fs.Path, []byte(content), 0600); err != nil {
		t.Fatalf("writing profiles.env: %v", err)
	}
	return fs
}

func TestFileStore_Exists(t *testing.T) {
	empty := t.TempDir()
	fs := NewFileStore(empty)
	if fs.Exists() {
		t.Error("Exists() = true for empty dir, want false")
	}

	fs2 := writeProfilesEnv(t, "NAME=x\nTOKEN=t\nREGION=us\n")
	if !fs2.Exists() {
		t.Error("Exists() = false after writing file, want true")
	}
}

func TestFileStore_SingleRecord(t *testing.T) {
	fs := writeProfilesEnv(t, `NAME=dev
WORKSPACE=acme-corp
ENVIRONMENT=dev
REGION=us
STORE_TYPE=file
BASE_URL=https://www.workato.com
TOKEN=wk-xxxxx
`)

	p, err := fs.GetProfile("dev")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p.Name != "dev" || p.Workspace != "acme-corp" || p.Environment != "dev" {
		t.Errorf("profile = %+v, wrong fields", p)
	}
	if p.Region != RegionUS {
		t.Errorf("Region = %q, want us", p.Region)
	}
	if p.StoreType != StoreFile {
		t.Errorf("StoreType = %q, want %q", p.StoreType, StoreFile)
	}

	cred, err := fs.Get(context.Background(), "dev")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cred.Token != "wk-xxxxx" {
		t.Errorf("Token = %q, want wk-xxxxx", cred.Token)
	}
	if cred.StoreType != StoreFile {
		t.Errorf("credential StoreType = %q, want %q", cred.StoreType, StoreFile)
	}
}

func TestFileStore_MultipleRecords(t *testing.T) {
	fs := writeProfilesEnv(t, `NAME=dev
WORKSPACE=acme-corp
ENVIRONMENT=dev
REGION=us
TOKEN=wk-dev

NAME=prod
WORKSPACE=acme-corp
ENVIRONMENT=prod
REGION=us
TOKEN=wk-prod
`)

	names, err := fs.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 || names[0] != "dev" || names[1] != "prod" {
		t.Errorf("List = %v, want [dev prod]", names)
	}

	prod, err := fs.GetProfile("prod")
	if err != nil {
		t.Fatalf("GetProfile(prod): %v", err)
	}
	if prod.Environment != "prod" {
		t.Errorf("prod.Environment = %q, want prod", prod.Environment)
	}
}

func TestFileStore_BaseURLDefaultsFromRegion(t *testing.T) {
	cases := []struct {
		region string
		want   string
	}{
		{"eu", "https://app.eu.workato.com"},
		{"il", "https://app.il.workato.com"},
		// CN uses a distinct domain (.workatoapp.cn) per Workato's allowlist docs.
		{"cn", "https://app.workatoapp.cn"},
	}
	for _, tc := range cases {
		t.Run(tc.region, func(t *testing.T) {
			fs := writeProfilesEnv(t, "NAME="+tc.region+"\nREGION="+tc.region+"\nTOKEN=t\n")
			p, err := fs.GetProfile(tc.region)
			if err != nil {
				t.Fatalf("GetProfile: %v", err)
			}
			if p.BaseURL != tc.want {
				t.Errorf("BaseURL = %q, want %q", p.BaseURL, tc.want)
			}
		})
	}
}

func TestFileStore_CommentsAndBlankLines(t *testing.T) {
	fs := writeProfilesEnv(t, `# header comment
# another comment

NAME=dev
# inline comment
REGION=us
TOKEN=t

`)
	p, err := fs.GetProfile("dev")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p.Name != "dev" {
		t.Errorf("Name = %q, want dev", p.Name)
	}
}

func TestFileStore_QuotedValues(t *testing.T) {
	fs := writeProfilesEnv(t, `NAME="dev"
REGION="us"
TOKEN="wk-xxxxx with spaces"
`)
	cred, err := fs.Get(context.Background(), "dev")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cred.Token != "wk-xxxxx with spaces" {
		t.Errorf("Token = %q, want stripped quotes and preserved inner spaces", cred.Token)
	}
}

func TestFileStore_SetAndDeleteAreReadOnly(t *testing.T) {
	fs := writeProfilesEnv(t, "NAME=x\nREGION=us\nTOKEN=t\n")
	if err := fs.Set(context.Background(), "y", &Credential{}); err == nil {
		t.Error("Set should return read-only error, got nil")
	}
	if err := fs.Delete(context.Background(), "x"); err == nil {
		t.Error("Delete should return read-only error, got nil")
	}
}

func TestFileStore_GetProfileNotFound(t *testing.T) {
	fs := writeProfilesEnv(t, "NAME=dev\nREGION=us\nTOKEN=t\n")
	if _, err := fs.GetProfile("nope"); err == nil {
		t.Error("GetProfile for missing name should error, got nil")
	}
}

func TestFileStore_MissingFile(t *testing.T) {
	fs := NewFileStore(t.TempDir())
	names, err := fs.List(context.Background())
	if err != nil {
		t.Fatalf("List on missing file should be empty, got error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("List = %v, want empty", names)
	}
}

func TestParseProfilesEnv_ErrorCases(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantSub string
	}{
		{
			name:    "field before NAME",
			input:   "TOKEN=t\nNAME=x\nREGION=us\n",
			wantSub: "appears before any NAME=",
		},
		{
			name:    "duplicate key",
			input:   "NAME=x\nREGION=us\nREGION=eu\nTOKEN=t\n",
			wantSub: "duplicate REGION=",
		},
		{
			name:    "missing TOKEN",
			input:   "NAME=x\nREGION=us\n",
			wantSub: "missing required field TOKEN",
		},
		{
			name:    "missing REGION",
			input:   "NAME=x\nTOKEN=t\n",
			wantSub: "missing required field REGION",
		},
		{
			name:    "bad STORE_TYPE",
			input:   "NAME=x\nREGION=us\nTOKEN=t\nSTORE_TYPE=keychain\n",
			wantSub: "STORE_TYPE=\"keychain\" invalid",
		},
		{
			name:    "no equals",
			input:   "NAME=x\nREGION us\nTOKEN=t\n",
			wantSub: "expected KEY=VALUE",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseProfilesEnv(strings.NewReader(tc.input))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestParseProfilesEnv_UnknownKeyWarns(t *testing.T) {
	recs, err := parseProfilesEnv(strings.NewReader("NAME=x\nREGION=us\nTOKEN=t\nFOO=bar\n"))
	if err != nil {
		t.Fatalf("unknown keys should warn, not error: %v", err)
	}
	if len(recs) != 1 || recs[0].name != "x" {
		t.Errorf("records = %+v, want one record named x", recs)
	}
}
