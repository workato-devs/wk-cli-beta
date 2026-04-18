package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// seedKeychainWithActive writes a keychain profile and sets it active.
func seedKeychainWithActive(t *testing.T, p *auth.Profile) {
	t.Helper()
	seedKeychainProfile(t, p, true)
}

// writeProjectWithSnapshot writes .wk/wk.toml at cwd with the given profile
// binding and snapshot fields.
func writeProjectWithSnapshot(t *testing.T, cwd string, cfg *config.Config) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(cwd, config.ProjectDir), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := config.Save(config.ProjectConfigPath(cwd), cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestRefreshSnapshotIfStale_NoOpWhenFresh(t *testing.T) {
	cwd := setupIsolatedHome(t)
	cfg := &config.Config{
		Name:        "p",
		Profile:     "prod",
		Workspace:   "Acme",
		WorkspaceID: 42,
		Environment: "dev",
		Email:       "u@x.com",
	}
	writeProjectWithSnapshot(t, cwd, cfg)

	profile := &auth.Profile{
		Name: "prod", Workspace: "Acme", WorkspaceID: 42, Environment: "dev", Email: "u@x.com",
	}
	changed, err := refreshSnapshotIfStale(cwd, cfg, profile)
	if err != nil {
		t.Fatalf("refreshSnapshotIfStale: %v", err)
	}
	if changed {
		t.Errorf("changed = true, want false when snapshot matches profile")
	}
}

func TestRefreshSnapshotIfStale_RewritesStale(t *testing.T) {
	cwd := setupIsolatedHome(t)
	cfg := &config.Config{
		Name:        "p",
		Profile:     "prod",
		Workspace:   "OldWorkspace",
		WorkspaceID: 1,
		Environment: "stage",
		Email:       "old@x.com",
	}
	writeProjectWithSnapshot(t, cwd, cfg)

	profile := &auth.Profile{
		Name: "prod", Workspace: "NewWorkspace", WorkspaceID: 99, Environment: "prod", Email: "new@x.com",
	}
	changed, err := refreshSnapshotIfStale(cwd, cfg, profile)
	if err != nil {
		t.Fatalf("refreshSnapshotIfStale: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true when snapshot is stale")
	}

	reloaded, err := config.Load(config.ProjectConfigPath(cwd))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Workspace != "NewWorkspace" || reloaded.WorkspaceID != 99 {
		t.Errorf("workspace snapshot not persisted: %+v", reloaded)
	}
	if reloaded.Environment != "prod" || reloaded.Email != "new@x.com" {
		t.Errorf("environment/email snapshot not persisted: %+v", reloaded)
	}
}

func TestRefreshSnapshotIfStale_NilInputs(t *testing.T) {
	if changed, err := refreshSnapshotIfStale("", nil, nil); changed || err != nil {
		t.Errorf("nil inputs: changed=%v err=%v, want false,nil", changed, err)
	}
	if changed, err := refreshSnapshotIfStale("/tmp", &config.Config{}, nil); changed || err != nil {
		t.Errorf("nil profile: changed=%v err=%v, want false,nil", changed, err)
	}
}

func TestAuthSwitch_InsideProject_RewritesProfileAndSnapshot(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)

	// Seed a project bound to "prod" with snapshot fields from "prod".
	writeProjectWithSnapshot(t, cwd, &config.Config{
		Name:        "p",
		Profile:     "prod",
		Workspace:   "OldWS",
		WorkspaceID: 1,
		Environment: "stage",
		Email:       "prod@x.com",
	})

	// Seed two keychain profiles; start active on "prod".
	seedKeychainProfile(t, &auth.Profile{
		Name: "prod", Workspace: "OldWS", WorkspaceID: 1, Environment: "stage", Email: "prod@x.com",
		Region: auth.RegionUS, StoreType: auth.StoreKeychain, BaseURL: "https://www.workato.com",
	}, true)
	// Append dev profile to profiles.json.
	pm := auth.NewProfileManager()
	if err := pm.SaveProfile(&auth.Profile{
		Name: "dev", Workspace: "DevWS", WorkspaceID: 9, Environment: "dev", Email: "dev@x.com",
		Region: auth.RegionUS, StoreType: auth.StoreKeychain, BaseURL: "https://www.workato.com",
	}); err != nil {
		t.Fatalf("upsert dev: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newAuthCmd())
	root.SetArgs([]string{"auth", "switch", "dev"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth switch dev: %v", err)
	}

	reloaded, err := config.Load(config.ProjectConfigPath(cwd))
	if err != nil {
		t.Fatalf("reload wk.toml: %v", err)
	}
	if reloaded.Profile != "dev" {
		t.Errorf("cfg.Profile = %q, want dev", reloaded.Profile)
	}
	if reloaded.Workspace != "DevWS" || reloaded.WorkspaceID != 9 {
		t.Errorf("snapshot not refreshed: %+v", reloaded)
	}
	if reloaded.Environment != "dev" || reloaded.Email != "dev@x.com" {
		t.Errorf("environment/email not refreshed: %+v", reloaded)
	}
}

func TestAuthSwitch_OutsideProject_OnlyUpdatesActive(t *testing.T) {
	resetGlobalFlags(t)
	setupIsolatedHome(t) // cwd is empty — no wk.toml
	seedKeychainWithActive(t, &auth.Profile{
		Name: "a", Workspace: "WS", Region: auth.RegionUS, StoreType: auth.StoreKeychain,
		BaseURL: "https://www.workato.com",
	})
	pm := auth.NewProfileManager()
	if err := pm.SaveProfile(&auth.Profile{
		Name: "b", Workspace: "WS2", Region: auth.RegionUS, StoreType: auth.StoreKeychain,
		BaseURL: "https://www.workato.com",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	root := NewRootCmd()
	root.AddCommand(newAuthCmd())
	root.SetArgs([]string{"auth", "switch", "b"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth switch b: %v", err)
	}

	// Ensure no wk.toml was created.
	if _, err := os.Stat(config.ProjectConfigPath(".")); !os.IsNotExist(err) {
		t.Errorf("wk.toml should not exist outside a project; stat err = %v", err)
	}
	active, _ := pm.GetActiveProfile()
	if active != "b" {
		t.Errorf("active profile = %q, want b", active)
	}
}

// Regression: before #33, a stale snapshot for a matching cfg.Profile would
// silently persist. resolveAPIClient now self-heals via refreshSnapshotIfStale.
func TestCheckProfileMatch_StillPassesAfterRefresh(t *testing.T) {
	cfg := &config.Config{Profile: "prod", Workspace: "OldWS"}
	if err := checkProfileMatch(cfg, "prod"); err != nil {
		t.Fatalf("mismatch with same profile: %v", err)
	}
}

// File-store-bound projects self-heal via resolveAPIClient even though the
// caller MUST pass --profile X --store-type file (explicit). Before the
// follow-up, the explicitProfile gate blocked this path and file-store
// projects had no reachable self-heal trigger.
func TestResolveAPIClient_FileStoreBound_SelfHeals(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)

	// Seed .wk/ with a file-store profile "academy" whose record has fresh
	// workspace/environment values...
	if err := os.MkdirAll(filepath.Join(cwd, config.ProjectDir), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	profilesEnv := "NAME=academy\nREGION=us\nWORKSPACE=NewWorkspace\nWORKSPACE_ID=222\nENVIRONMENT=prod\nEMAIL=new@x.com\nTOKEN=tok\n"
	if err := os.WriteFile(auth.NewFileStore(cwd).Path, []byte(profilesEnv), 0600); err != nil {
		t.Fatalf("write profiles.env: %v", err)
	}

	// ...but wk.toml's snapshot reflects a stale prior workspace.
	if err := config.Save(config.ProjectConfigPath(cwd), &config.Config{
		Name:        "p",
		Profile:     "academy",
		Workspace:   "OldWorkspace",
		WorkspaceID: 111,
		Environment: "stage",
		Email:       "old@x.com",
	}); err != nil {
		t.Fatalf("save wk.toml: %v", err)
	}

	// Invoke the way a file-store project must be invoked.
	flagProfile = "academy"
	flagStoreType = string(auth.StoreFile)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	_, resolved, err := resolveAPIClient(cmd)
	if err != nil {
		t.Fatalf("resolveAPIClient: %v", err)
	}
	if resolved.Name != "academy" {
		t.Errorf("resolved.Name = %q, want academy", resolved.Name)
	}

	reloaded, err := config.Load(config.ProjectConfigPath(cwd))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Workspace != "NewWorkspace" || reloaded.WorkspaceID != 222 {
		t.Errorf("file-store snapshot not self-healed: %+v", reloaded)
	}
	if reloaded.Environment != "prod" || reloaded.Email != "new@x.com" {
		t.Errorf("environment/email not self-healed: %+v", reloaded)
	}
}

// Cross-profile explicit --profile (intent override) must NOT mutate the
// project's snapshot — the user is running against a different profile
// one-off, not rebinding the project.
func TestResolveAPIClient_ExplicitCrossProfile_DoesNotRewrite(t *testing.T) {
	resetGlobalFlags(t)
	cwd := setupIsolatedHome(t)

	// Project is bound to "academy" (file-store) with its own snapshot.
	if err := os.MkdirAll(filepath.Join(cwd, config.ProjectDir), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	profilesEnv := "NAME=academy\nREGION=us\nWORKSPACE=AcademyWS\nTOKEN=tok\n" +
		"NAME=other\nREGION=us\nWORKSPACE=OtherWS\nTOKEN=tok2\n"
	if err := os.WriteFile(auth.NewFileStore(cwd).Path, []byte(profilesEnv), 0600); err != nil {
		t.Fatalf("write profiles.env: %v", err)
	}
	if err := config.Save(config.ProjectConfigPath(cwd), &config.Config{
		Name:      "p",
		Profile:   "academy",
		Workspace: "AcademyWS",
	}); err != nil {
		t.Fatalf("save wk.toml: %v", err)
	}

	// User explicitly runs against "other" — a different profile.
	flagProfile = "other"
	flagStoreType = string(auth.StoreFile)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if _, _, err := resolveAPIClient(cmd); err != nil {
		t.Fatalf("resolveAPIClient: %v", err)
	}

	reloaded, err := config.Load(config.ProjectConfigPath(cwd))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Profile != "academy" || reloaded.Workspace != "AcademyWS" {
		t.Errorf("cross-profile invocation mutated project state: %+v", reloaded)
	}
}
