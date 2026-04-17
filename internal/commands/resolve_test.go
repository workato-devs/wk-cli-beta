package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
)

func TestCheckProfileMatch_Mismatch(t *testing.T) {
	cfg := &config.Config{Profile: "prod"}
	err := checkProfileMatch(cfg, "dev")
	if err == nil {
		t.Fatal("expected error for profile mismatch")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"dev"`) || !strings.Contains(msg, `"prod"`) {
		t.Errorf("error should mention both profiles, got: %s", msg)
	}
	if !strings.Contains(msg, "wk auth switch prod") {
		t.Errorf("error should suggest auth switch, got: %s", msg)
	}
}

func TestCheckProfileMatch_Match(t *testing.T) {
	cfg := &config.Config{Profile: "prod"}
	err := checkProfileMatch(cfg, "prod")
	if err != nil {
		t.Fatalf("unexpected error for matching profile: %v", err)
	}
}

func TestCheckProfileMatch_EmptyProfile(t *testing.T) {
	cfg := &config.Config{Profile: ""}
	err := checkProfileMatch(cfg, "anything")
	if err != nil {
		t.Fatalf("unexpected error when profile is empty: %v", err)
	}
}

// resetGlobalFlags restores the global flag state after a test mutates it.
func resetGlobalFlags(t *testing.T) {
	t.Helper()
	origStore := flagStoreType
	origProfile := flagProfile
	t.Cleanup(func() {
		flagStoreType = origStore
		flagProfile = origProfile
	})
}

// setupIsolatedHome creates a temp HOME dir and changes into a temp cwd.
// Returns the cwd path; HOME is set via t.Setenv. Caller can optionally
// write a wk.toml + profiles.env into cwd.
func setupIsolatedHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	os.MkdirAll(filepath.Join(tmpHome, ".wk"), 0700)

	cwd := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	return cwd
}

// writeKeychainProfile seeds ~/.wk/profiles.json with one profile.
func writeKeychainProfile(t *testing.T, p *auth.Profile) {
	t.Helper()
	data, _ := json.Marshal([]*auth.Profile{p})
	path := filepath.Join(os.Getenv("HOME"), ".wk", "profiles.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writing profiles.json: %v", err)
	}
}

// writeProjectFileStore creates wk.toml + profiles.env at cwd for a named
// profile.
func writeProjectFileStore(t *testing.T, cwd, name, token string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cwd, config.ProjectFile),
		[]byte(`name = "test"`+"\n"), 0644); err != nil {
		t.Fatalf("writing wk.toml: %v", err)
	}
	body := "NAME=" + name + "\nREGION=us\nWORKSPACE=acme\nENVIRONMENT=dev\nTOKEN=" + token + "\n"
	if err := os.WriteFile(filepath.Join(cwd, auth.ProfilesEnvFile),
		[]byte(body), 0600); err != nil {
		t.Fatalf("writing profiles.env: %v", err)
	}
}

func TestResolveProfileAndCred_UnknownStoreType(t *testing.T) {
	resetGlobalFlags(t)
	setupIsolatedHome(t)
	flagStoreType = "bogus"

	_, _, err := resolveProfileAndCred(context.Background(), "any")
	if err == nil || !strings.Contains(err.Error(), "unknown --store-type") {
		t.Errorf("err = %v, want 'unknown --store-type'", err)
	}
}

func TestResolveProfileAndCred_FileStoreExplicit(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectFileStore(t, cwd, "ci", "tok-123")
	flagStoreType = string(auth.StoreFile)

	profile, cred, err := resolveProfileAndCred(context.Background(), "ci")
	if err != nil {
		t.Fatalf("resolveProfileAndCred: %v", err)
	}
	if profile.Name != "ci" || profile.StoreType != auth.StoreFile {
		t.Errorf("profile = %+v, want name=ci store=file", profile)
	}
	if cred.Token != "tok-123" {
		t.Errorf("token = %q, want tok-123", cred.Token)
	}
}

func TestResolveProfileAndCred_FileStoreMissingFile(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	// Create wk.toml but NO profiles.env.
	os.WriteFile(filepath.Join(cwd, config.ProjectFile), []byte(`name = "test"`), 0644)
	flagStoreType = string(auth.StoreFile)

	_, _, err := resolveProfileAndCred(context.Background(), "ci")
	if err == nil || !strings.Contains(err.Error(), "no profiles.env found") {
		t.Errorf("err = %v, want 'no profiles.env found'", err)
	}
}

func TestResolveProfileAndCred_FileStoreOutsideProject(t *testing.T) {
	resetGlobalFlags(t)
	setupIsolatedHome(t) // cwd has no wk.toml
	flagStoreType = string(auth.StoreFile)

	_, _, err := resolveProfileAndCred(context.Background(), "ci")
	if err == nil || !strings.Contains(err.Error(), "requires a project directory") {
		t.Errorf("err = %v, want 'requires a project directory'", err)
	}
}

func TestResolveProfileAndCred_ImplicitFallthroughToFile(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)
	writeProjectFileStore(t, cwd, "ci", "tok-from-file")
	// No profile in profiles.json; implicit routing should find it in
	// profiles.env.

	profile, cred, err := resolveProfileAndCred(context.Background(), "ci")
	if err != nil {
		t.Fatalf("resolveProfileAndCred: %v", err)
	}
	if profile.StoreType != auth.StoreFile {
		t.Errorf("StoreType = %q, want file", profile.StoreType)
	}
	if cred.Token != "tok-from-file" {
		t.Errorf("token = %q, want tok-from-file", cred.Token)
	}
}

func TestResolveProfileAndCred_ImplicitBothMiss(t *testing.T) {
	resetGlobalFlags(t)
	setupIsolatedHome(t)
	// Neither profiles.json nor profiles.env contains "ghost".

	_, _, err := resolveProfileAndCred(context.Background(), "ghost")
	if err == nil || !strings.Contains(err.Error(), "not found in profiles.json or profiles.env") {
		t.Errorf("err = %v, want both-miss error", err)
	}
}

func TestResolveProfileAndCred_KeychainProfileButFileType(t *testing.T) {
	resetGlobalFlags(t)
	setupIsolatedHome(t) // no wk.toml
	writeKeychainProfile(t, &auth.Profile{
		Name:      "weird",
		Region:    auth.RegionUS,
		StoreType: auth.StoreFile, // profile says "file" but we're outside a project
		BaseURL:   "https://www.workato.com",
	})

	_, _, err := resolveProfileAndCred(context.Background(), "weird")
	if err == nil || !strings.Contains(err.Error(), "store_type=file") {
		t.Errorf("err = %v, want store_type=file referenced", err)
	}
}
