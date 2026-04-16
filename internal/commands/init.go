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
// init --verify. This resolves region from the profile rather than relying
// on the global resolveAPIClient path.
func resolveVerifyClient(cmd *cobra.Command, profileName string) (api.Client, error) {
	pm := auth.NewProfileManager()
	profile, err := pm.GetProfile(profileName)
	if err != nil {
		return nil, fmt.Errorf("profile %q not found — run 'wk auth login' first", profileName)
	}

	store := auth.NewChainStore(&auth.EnvStore{}, &auth.KeyringStore{})
	cred, err := store.Get(cmd.Context(), profileName)
	if err != nil {
		return nil, fmt.Errorf("no credentials for profile %q: %w", profileName, err)
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
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new wk project",
		Long:  "Create a new wk project directory with a wk.toml config file.",
		Args:  cobra.NoArgs,
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

			if flagJSON {
				// Non-interactive: require --name and --profile.
				if name == "" {
					return fmt.Errorf("--name is required in non-interactive (--json) mode")
				}
				if profile == "" {
					return fmt.Errorf("--profile is required in non-interactive (--json) mode")
				}
			} else {
				// Interactive: prompt for missing values.
				reader := bufio.NewReader(os.Stdin)
				if name == "" {
					fmt.Print("Project name: ")
					name, _ = reader.ReadString('\n')
					name = strings.TrimSpace(name)
					if name == "" {
						return fmt.Errorf("project name is required")
					}
				}
				if profile == "" {
					fmt.Print("Auth profile: ")
					profile, _ = reader.ReadString('\n')
					profile = strings.TrimSpace(profile)
					if profile == "" {
						return fmt.Errorf("auth profile is required")
					}
				}
			}

			// Resolve the target directory: <cwd>/<name>/
			targetDir := filepath.Join(cwd, name)
			configPath := filepath.Join(targetDir, config.ProjectFile)

			// Check if target already contains a wk.toml.
			if _, err := os.Stat(configPath); err == nil {
				return fmt.Errorf("project %q already exists at %s", name, targetDir)
			}

			// Create the container directory (no-op if it already exists).
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("creating project directory: %w", err)
			}

			cfg := &config.Config{
				Name:    name,
				Profile: profile,
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

	return cmd
}
