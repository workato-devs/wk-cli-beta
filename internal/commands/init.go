package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

// resolveVerifyClient builds an API client for the named profile, used by
// init --verify. This honors StoreType routing (ADR-006 Sub-decision 6) via
// the shared resolveProfileAndCred helper.
func resolveVerifyClient(cmd *cobra.Command, profileName string) (api.Client, error) {
	profile, cred, err := resolveProfileAndCred(cmd.Context(), profileName)
	if err != nil {
		return nil, err
	}

	var opts []api.ClientOption
	if flagVerbose {
		opts = append(opts, api.WithVerbose(true))
	}

	client := api.NewHTTPClient(profile.BaseURL+config.APIPathPrefix, cred.Token, opts...)
	return client, nil
}

// verifyServerPath walks the Workato folder hierarchy to confirm that
// serverPath exists. Returns nil on success or a descriptive error.
func verifyServerPath(cmd *cobra.Command, client api.Client, serverPath string) error {
	parts := strings.Split(strings.Trim(serverPath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("empty server path")
	}

	// Strip implicit root folder "All projects" if present.
	if strings.EqualFold(parts[0], "All projects") {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return nil // root folder always exists
	}

	folders := client.Folders()

	// Fetch the full folder list once (unfiltered) so we can resolve
	// top-level workspace folders whose parent_id is the implicit home
	// folder — the API never returns the home folder itself, so filtering
	// by parent_id=nil would find nothing.
	allFolders, err := folders.List(cmd.Context(), nil)
	if err != nil {
		return fmt.Errorf("verifying server path: %w", err)
	}

	var parentID *int
	for _, name := range parts {
		found := false
		for _, f := range allFolders {
			if !strings.EqualFold(f.Name, name) {
				continue
			}
			// For the first segment, match by name only (top-level folders
			// sit under the implicit home folder whose ID we don't know).
			// For deeper segments, also require the parent ID to match.
			if parentID != nil && (f.ParentID == nil || *f.ParentID != *parentID) {
				continue
			}
			id := f.ID
			parentID = &id
			found = true
			break
		}
		if !found {
			return fmt.Errorf("server path %q not found: folder %q does not exist", serverPath, name)
		}
	}
	return nil
}

func newInitCmd() *cobra.Command {
	var (
		flagName        string
		flagInitProfile string
		flagServerPath  string
		flagLocalPath   string
		flagVerify      bool
		flagInitNoInput bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new wk project",
		Long: `Create a new wk project directory with a wk.toml config file.

Non-interactive mode (detected via --json, --no-input, or a non-TTY stdin)
requires --name and --profile explicitly. Mirrors the contract used by
'wk auth login'.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}

			// Nesting guard: prevent creating a project inside an existing one.
			if projectRoot, err := config.FindProjectRoot(cwd); err == nil {
				return fmt.Errorf("%w at %s — run from outside the project directory", wkerrors.ErrNestedProject, projectRoot)
			}

			name := flagName
			profile := flagInitProfile
			interactive := isInteractiveStdin() && !flagInitNoInput && !flagJSON

			// In non-interactive mode, validate required flags upfront so
			// no prompt label ever reaches the terminal (mirrors auth login).
			if !interactive {
				var missing []string
				if name == "" {
					missing = append(missing, "--name")
				}
				if profile == "" {
					missing = append(missing, "--profile")
				}
				if len(missing) > 0 {
					return fmt.Errorf("%s required in non-interactive mode (detected via --json, --no-input, or non-TTY stdin)",
						strings.Join(missing, " and "))
				}
			} else {
				reader := bufio.NewReader(os.Stdin)
				if name == "" {
					fmt.Print("Project name: ")
					name, _ = reader.ReadString('\n')
					name = strings.TrimSpace(name)
					if name == "" {
						return fmt.Errorf("project name cannot be empty")
					}
				}
				if profile == "" {
					fmt.Print("Auth profile: ")
					profile, _ = reader.ReadString('\n')
					profile = strings.TrimSpace(profile)
					if profile == "" {
						return fmt.Errorf("auth profile cannot be empty")
					}
				}
			}

			// Resolve the target directory: <cwd>/<name>/
			targetDir := filepath.Join(cwd, name)
			configPath := filepath.Join(targetDir, config.ProjectFile)

			// Validate profile according to --store-type. File-store profiles
			// live in the target directory's profiles.env; keychain profiles
			// live in the user-level profiles.json and must match the active
			// profile.
			var resolvedProfile *auth.Profile
			switch flagStoreType {
			case "", string(auth.StoreKeychain):
				pm := auth.NewProfileManager()
				p, err := pm.GetProfile(profile)
				if err != nil {
					return fmt.Errorf("profile %q not found — run 'wk auth login' first", profile)
				}
				if activeName, err := pm.GetActiveProfile(); err == nil && activeName != profile {
					return fmt.Errorf("active profile %q does not match target profile %q", activeName, profile)
				}
				resolvedProfile = p
			case string(auth.StoreFile):
				envPath := filepath.Join(targetDir, auth.ProfilesEnvFile)
				if _, err := os.Stat(envPath); err != nil {
					if !os.IsNotExist(err) {
						return fmt.Errorf("stat %s: %w", envPath, err)
					}
					fmt.Fprintf(os.Stderr,
						"warning: --store-type file specified but no %s found at %s — create one before running commands\n",
						auth.ProfilesEnvFile, envPath)
					// resolvedProfile stays nil; snapshot fields will be
					// omitted from wk.toml (omitempty).
				} else {
					fs := auth.NewFileStore(targetDir)
					p, ferr := fs.GetProfile(profile)
					if ferr != nil {
						return fmt.Errorf("profile %q not found in %s", profile, envPath)
					}
					resolvedProfile = p
				}
			default:
				return fmt.Errorf("unknown --store-type %q; valid: %s, %s",
					flagStoreType, auth.StoreKeychain, auth.StoreFile)
			}

			// Check if target already contains a wk.toml.
			if _, err := os.Stat(configPath); err == nil {
				return fmt.Errorf("project %q already exists at %s", name, targetDir)
			}

			// Create the container directory (no-op if it already exists).
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("creating project directory: %w", err)
			}

			// Snapshot workspace/environment/email from the resolved profile
			// into wk.toml per ADR-006 Sub-decision 8. These fields are
			// informational only — runtime routing always uses the profile
			// store. Safe to persist because .wk/ is gitignored (ADR-005).
			// resolvedProfile may be nil in --store-type file deferred mode,
			// in which case omitempty keeps the fields out of wk.toml.
			cfg := &config.Config{
				Name:    name,
				Profile: profile,
			}
			if resolvedProfile != nil {
				cfg.Workspace = resolvedProfile.Workspace
				cfg.WorkspaceID = resolvedProfile.WorkspaceID
				cfg.Environment = resolvedProfile.Environment
				cfg.Email = resolvedProfile.Email
			}

			if flagServerPath != "" {
				localPath := flagLocalPath
				if localPath == "" {
					localPath = "."
				}

				if flagVerify {
					client, err := resolveVerifyClient(cmd, profile)
					if err != nil {
						return fmt.Errorf("--verify requires auth: %w", err)
					}
					if err := verifyServerPath(cmd, client, flagServerPath); err != nil {
						return err
					}
				}

				cfg.Sync = []config.SyncEntry{
					{
						ServerPath: flagServerPath,
						LocalPath:  localPath,
					},
				}
			}

			if err := config.Save(configPath, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			result := map[string]string{
				"status":  "initialized",
				"name":    name,
				"profile": profile,
				"path":    configPath,
			}

			if flagJSON {
				return rctx.Formatter.Format(cmd.OutOrStdout(), result)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Initialized wk project %q (profile: %s) at %s\n", name, profile, configPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&flagName, "name", "", "Project name (also the container directory name)")
	cmd.Flags().StringVar(&flagInitProfile, "profile", "", "Auth profile name")
	cmd.Flags().StringVar(&flagServerPath, "server-path", "", "Initial sync server path")
	cmd.Flags().StringVar(&flagLocalPath, "local-path", "", "Initial sync local path (defaults to \".\")")
	cmd.Flags().BoolVar(&flagVerify, "verify", false, "Validate server-path exists on Workato before saving")
	cmd.Flags().BoolVar(&flagInitNoInput, "no-input", false, "Force non-interactive mode (fail on missing required flags instead of prompting)")

	return cmd
}
