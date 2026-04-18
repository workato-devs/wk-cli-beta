package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// resolveAPIClient builds an authenticated API client from the active profile
// or --profile flag override, using StoreType-driven routing per ADR-006
// Sub-decision 6.
func resolveAPIClient(cmd *cobra.Command) (api.Client, *auth.Profile, error) {
	activeName, explicitProfile, err := resolveActiveProfileName()
	if err != nil {
		return nil, nil, err
	}

	profile, cred, err := resolveProfileAndCred(cmd.Context(), activeName)
	if err != nil {
		return nil, nil, err
	}

	// P0: profile isolation + snapshot self-heal.
	//
	// The match check is only enforced when --profile was NOT explicitly set
	// (explicit = intent override). The snapshot refresh is independent: it
	// runs whenever the resolved profile matches cfg.Profile, regardless of
	// how it was specified. For file-store-bound projects `--profile X
	// --store-type file` is the only way to invoke the project, not an
	// override, so gating self-heal on !explicitProfile would leave their
	// snapshots permanently stale (ADR-006 Sub-decision 3 forbids setting
	// file-store profiles as the global active, so wk auth switch trigger-2
	// is also unreachable for them).
	if cwd, err := os.Getwd(); err == nil {
		if projectRoot, err := config.FindProjectRoot(cwd); err == nil {
			if cfg, err := config.Load(config.ProjectConfigPath(projectRoot)); err == nil {
				if !explicitProfile {
					if err := checkProfileMatch(cfg, profile.Name); err != nil {
						return nil, nil, err
					}
				}
				if profile.Name == cfg.Profile {
					if refreshed, rerr := refreshSnapshotIfStale(projectRoot, cfg, profile); rerr != nil {
						fmt.Fprintf(os.Stderr, "warning: %v\n", rerr)
					} else if refreshed && flagVerbose {
						fmt.Fprintf(os.Stderr, "[debug] refreshed wk.toml snapshot for profile %q\n", profile.Name)
					}
				}
			}
		}
	}

	var opts []api.ClientOption
	if flagTimeout > 0 {
		opts = append(opts, api.WithTimeout(
			config.TimeoutDuration(flagTimeout),
		))
	}
	if flagVerbose {
		opts = append(opts, api.WithVerbose(true))
		fmt.Fprintf(os.Stderr, "[debug] profile=%s region=%s base_url=%s store=%s\n",
			profile.Name, profile.Region, profile.BaseURL, profile.StoreType)
	}

	client := api.NewHTTPClient(profile.BaseURL+config.APIPathPrefix, cred.Token, opts...)
	return client, profile, nil
}

// resolveActiveProfileName returns the profile name to use and whether it
// came from an explicit --profile flag.
func resolveActiveProfileName() (name string, explicit bool, err error) {
	pm := auth.NewProfileManager()
	active, _ := pm.GetActiveProfile()
	name = active
	if flagProfile != "" {
		name = flagProfile
		explicit = true
	}
	if name == "" {
		return "", false, fmt.Errorf("no active profile; run 'wk auth login' first or use --profile")
	}
	return name, explicit, nil
}

// resolveProfileAndCred returns the Profile and Credential for name, honoring
// --store-type overrides and the ADR-006 Sub-decision 6 routing order:
//
//  1. --store-type explicit → route to that backend directly.
//  2. profile in profiles.json → dispatch on profile.StoreType.
//  3. inside a project with profiles.env → implicit file-store lookup.
//  4. not found anywhere → error.
func resolveProfileAndCred(ctx context.Context, name string) (*auth.Profile, *auth.Credential, error) {
	switch flagStoreType {
	case "":
		return resolveImplicit(ctx, name)
	case string(auth.StoreKeychain):
		return lookupKeychain(ctx, name)
	case string(auth.StoreFile):
		return lookupFileStoreInProject(ctx, name)
	default:
		return nil, nil, fmt.Errorf("unknown --store-type %q; valid: %s, %s",
			flagStoreType, auth.StoreKeychain, auth.StoreFile)
	}
}

func resolveImplicit(ctx context.Context, name string) (*auth.Profile, *auth.Credential, error) {
	pm := auth.NewProfileManager()
	if profile, err := pm.GetProfile(name); err == nil {
		switch profile.StoreType {
		case auth.StoreFile:
			fs, ferr := fileStoreForCwd()
			if ferr != nil {
				return nil, nil, fmt.Errorf("profile %q has store_type=file but %w", name, ferr)
			}
			cred, cerr := fs.Get(ctx, name)
			if cerr != nil {
				return nil, nil, fmt.Errorf("no credentials for profile %q in profiles.env: %w", name, cerr)
			}
			return profile, cred, nil
		default:
			// keychain or legacy empty
			cred, cerr := (&auth.KeyringStore{}).Get(ctx, name)
			if cerr != nil {
				return nil, nil, fmt.Errorf("no credentials for profile %q: %w", name, cerr)
			}
			return profile, cred, nil
		}
	}

	if fs, err := fileStoreForCwd(); err == nil {
		if profile, perr := fs.GetProfile(name); perr == nil {
			cred, cerr := fs.Get(ctx, name)
			if cerr != nil {
				return nil, nil, fmt.Errorf("no credentials for profile %q in profiles.env: %w", name, cerr)
			}
			return profile, cred, nil
		}
	}

	return nil, nil, fmt.Errorf("profile %q not found in profiles.json or profiles.env", name)
}

func lookupKeychain(ctx context.Context, name string) (*auth.Profile, *auth.Credential, error) {
	pm := auth.NewProfileManager()
	profile, err := pm.GetProfile(name)
	if err != nil {
		return nil, nil, fmt.Errorf("profile %q not found in profiles.json", name)
	}
	cred, err := (&auth.KeyringStore{}).Get(ctx, name)
	if err != nil {
		return nil, nil, fmt.Errorf("no credentials for profile %q: %w", name, err)
	}
	return profile, cred, nil
}

func lookupFileStoreInProject(ctx context.Context, name string) (*auth.Profile, *auth.Credential, error) {
	fs, err := fileStoreForCwd()
	if err != nil {
		return nil, nil, err
	}
	profile, err := fs.GetProfile(name)
	if err != nil {
		return nil, nil, fmt.Errorf("profile %q not found in profiles.env at %s", name, fs.Path)
	}
	cred, err := fs.Get(ctx, name)
	if err != nil {
		return nil, nil, fmt.Errorf("no credentials for profile %q in profiles.env: %w", name, err)
	}
	return profile, cred, nil
}

// fileStoreForCwd locates the project root from the current working
// directory and returns a FileStore if profiles.env exists there. Errors
// when the caller is outside a project or when profiles.env is absent.
func fileStoreForCwd() (*auth.FileStore, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting current directory: %w", err)
	}
	projectRoot, err := config.FindProjectRoot(cwd)
	if err != nil {
		return nil, fmt.Errorf("--store-type file requires a project directory (.wk/wk.toml not found)")
	}
	fs := auth.NewFileStore(projectRoot)
	if !fs.Exists() {
		return nil, fmt.Errorf("no %s found at %s", auth.ProfilesEnvFile, fs.Path)
	}
	return fs, nil
}

// checkProfileMatch returns an error if the project config specifies a profile
// that doesn't match the active profile name.
func checkProfileMatch(cfg *config.Config, profileName string) error {
	if cfg.Profile != "" && cfg.Profile != profileName {
		return fmt.Errorf(
			"active profile %q does not match project profile %q\n"+
				"Use --profile %s or run: wk auth switch %s",
			profileName, cfg.Profile, cfg.Profile, cfg.Profile,
		)
	}
	return nil
}

// refreshSnapshotIfStale rewrites wk.toml's snapshot fields (workspace,
// workspace_id, environment, email) when they diverge from the resolved
// profile. Addresses issue #33 / ADR-006 Sub-decision 8 — the snapshot
// exists so `cat wk.toml` reveals what the project targets, and a stale
// snapshot actively misleads.
//
// Returns true when wk.toml was rewritten. Save failures are surfaced so
// callers can decide whether to warn or fail; most callers should warn —
// a stale snapshot does not prevent the current command from running.
func refreshSnapshotIfStale(projectRoot string, cfg *config.Config, profile *auth.Profile) (bool, error) {
	if cfg == nil || profile == nil || projectRoot == "" {
		return false, nil
	}
	if cfg.Workspace == profile.Workspace &&
		cfg.WorkspaceID == profile.WorkspaceID &&
		cfg.Environment == profile.Environment &&
		cfg.Email == profile.Email {
		return false, nil
	}
	cfg.Workspace = profile.Workspace
	cfg.WorkspaceID = profile.WorkspaceID
	cfg.Environment = profile.Environment
	cfg.Email = profile.Email
	if err := config.Save(config.ProjectConfigPath(projectRoot), cfg); err != nil {
		return false, fmt.Errorf("refreshing wk.toml snapshot: %w", err)
	}
	return true, nil
}
