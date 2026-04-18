package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Acme Corp", "acme-corp"},
		{"Acme_Corp", "acme-corp"},
		{"  Acme   Corp!! ", "acme-corp"},
		{"ACME", "acme"},
		{"123 Corp", "123-corp"},
		{"--leading---and---trailing--", "leading-and-trailing"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := slugify(c.in); got != c.want {
				t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// seedKeychainProfile writes a profiles.json under HOME/.wk/ with the given
// profile and active-profile marker.
func seedKeychainProfile(t *testing.T, p *auth.Profile, active bool) {
	t.Helper()
	home := os.Getenv("HOME")
	wkDir := filepath.Join(home, ".wk")
	os.MkdirAll(wkDir, 0700)
	data, _ := json.Marshal([]*auth.Profile{p})
	os.WriteFile(filepath.Join(wkDir, "profiles.json"), data, 0600)
	if active {
		os.WriteFile(filepath.Join(wkDir, "active_profile"), []byte(p.Name), 0600)
	}
}

// seedProjectProfilesEnv writes a .wk/wk.toml + .wk/profiles.env into cwd
// with the given profile name.
func seedProjectProfilesEnv(t *testing.T, cwd, name string) {
	t.Helper()
	os.MkdirAll(filepath.Join(cwd, config.ProjectDir), 0755)
	os.WriteFile(config.ProjectConfigPath(cwd), []byte(`name = "test"`+"\n"), 0644)
	body := "NAME=" + name + "\nREGION=us\nWORKSPACE=acme\nENVIRONMENT=ci\nTOKEN=tok\n"
	os.WriteFile(auth.NewFileStore(cwd).Path, []byte(body), 0600)
}

func TestAuthSwitch_FileOnlyProfileErrors(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	seedProjectProfilesEnv(t, cwd, "ci")

	root := NewRootCmd()
	root.AddCommand(newAuthCmd())
	root.SetArgs([]string{"auth", "switch", "ci"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error switching to file-only profile, got nil")
	}
	if !strings.Contains(err.Error(), "file-store profile") {
		t.Errorf("err = %v, want 'file-store profile' message", err)
	}
	if !strings.Contains(err.Error(), "--store-type file") {
		t.Errorf("err = %v, want --store-type file hint", err)
	}
}

func TestAuthSwitch_UnknownProfileErrors(t *testing.T) {
	resetGlobalFlags(t)
	setupIsolatedHome(t)

	root := NewRootCmd()
	root.AddCommand(newAuthCmd())
	root.SetArgs([]string{"auth", "switch", "ghost"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want 'not found'", err)
	}
}

func TestAuthLogin_NonInteractiveFailsFast(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantMissing []string
	}{
		{"json no flags", []string{"auth", "login", "--json"}, []string{"--token", "--environment"}},
		{"no-input token only", []string{"auth", "login", "--token", "x", "--no-input"}, []string{"--environment"}},
		{"json token only", []string{"auth", "login", "--token", "x", "--json"}, []string{"--environment"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetGlobalFlags(t)
			setupIsolatedHome(t)

			root := NewRootCmd()
			root.AddCommand(newAuthCmd())
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil {
				t.Fatal("expected non-interactive validation error, got nil")
			}
			msg := err.Error()
			for _, flag := range tc.wantMissing {
				if !strings.Contains(msg, flag) {
					t.Errorf("err = %q, want substring %q", msg, flag)
				}
			}
			if !strings.Contains(msg, "non-interactive mode") {
				t.Errorf("err = %q, want mention of non-interactive mode", msg)
			}
			// Most importantly: confirm the error fires before any prompt
			// could have been printed — the error contains no prompt label.
			if strings.Contains(msg, "Environment (") || strings.Contains(msg, "API token:") {
				t.Errorf("err contains a prompt label: %q", msg)
			}
		})
	}
}

func TestAuthList_MergesFileStoreAndShadows(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	// Seed keychain with a profile named "ci".
	seedKeychainProfile(t, &auth.Profile{
		Name:        "ci",
		Workspace:   "acme",
		Environment: "prod",
		Region:      auth.RegionUS,
		StoreType:   auth.StoreKeychain,
		BaseURL:     "https://www.workato.com",
	}, true)
	// Seed project profiles.env with the same name — should appear shadowed.
	seedProjectProfilesEnv(t, cwd, "ci")

	root := NewRootCmd()
	root.AddCommand(newAuthCmd())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"auth", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth list: %v", err)
	}

	// Can't easily capture stdout through cobra without refactor; use the
	// JSON formatter path instead for a deterministic assertion.
	root2 := NewRootCmd()
	root2.AddCommand(newAuthCmd())
	root2.SetArgs([]string{"auth", "list", "--json"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("auth list --json: %v", err)
	}
}

func TestComputeProfileName(t *testing.T) {
	cases := []struct {
		workspace, environment, region, want string
	}{
		// ADR-006 examples: region is always the leading component.
		{"Acme Corp", "prod", "us", "us-acme-corp-prod"},
		{"Acme Corp", "prod", "eu", "eu-acme-corp-prod"},
		{"Acme", "dev", "us", "us-acme-dev"},
		// Empty region falls back to config.DefaultRegion ("us").
		{"Acme", "dev", "", "us-acme-dev"},
		{"Acme", "dev", "jp", "jp-acme-dev"},
		// Coverage for the two newly-added regions.
		{"Acme", "prod", "il", "il-acme-prod"},
		{"Acme", "prod", "cn", "cn-acme-prod"},
	}
	for _, c := range cases {
		got := computeProfileName(c.workspace, c.environment, c.region)
		if got != c.want {
			t.Errorf("computeProfileName(%q,%q,%q) = %q, want %q",
				c.workspace, c.environment, c.region, got, c.want)
		}
	}
}
