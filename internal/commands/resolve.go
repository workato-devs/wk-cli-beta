package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

	// P0: Profile isolation check — prevent accidental cross-environment operations.
	// Only enforced when --profile was NOT explicitly set (explicit = intent override).
	if !explicitProfile {
		if cwd, err := os.Getwd(); err == nil {
			if projectRoot, err := config.FindProjectRoot(cwd); err == nil {
				if cfg, err := config.Load(filepath.Join(projectRoot, config.ProjectFile)); err == nil {
					if err := checkProfileMatch(cfg, profile.Name); err != nil {
						return nil, nil, err
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
		return nil, fmt.Errorf("--store-type file requires a project directory (wk.toml not found)")
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
