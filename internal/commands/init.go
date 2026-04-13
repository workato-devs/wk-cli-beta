package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/config"
)

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
		flagName       string
		flagProfile    string
		flagServerPath string
		flagLocalPath  string
		flagVerify     bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new wk project in the current directory",
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

			// Check if wk.toml already exists in the current directory.
			configPath := filepath.Join(cwd, config.ProjectFile)
			if _, err := os.Stat(configPath); err == nil {
				return fmt.Errorf("wk.toml already exists. Use 'wk link' to update the linked profile.")
			}

			name := flagName
			profile := flagProfile

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

			cfg := &config.Config{
				Name:    name,
				Profile: profile,
			}

			if flagServerPath != "" {
				localPath := flagLocalPath
				if localPath == "" {
					// Default to "./<leaf of server path>" so local folders mirror server hierarchy.
					parts := strings.Split(strings.Trim(flagServerPath, "/"), "/")
					localPath = "./" + parts[len(parts)-1]
				}

				if flagVerify {
					client, _, err := resolveAPIClient(cmd)
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

	cmd.Flags().StringVar(&flagName, "name", "", "Project name")
	cmd.Flags().StringVar(&flagProfile, "profile", "", "Auth profile name")
	cmd.Flags().StringVar(&flagServerPath, "server-path", "", "Initial sync server path")
	cmd.Flags().StringVar(&flagLocalPath, "local-path", "", "Initial sync local path (defaults to server-path leaf)")
	cmd.Flags().BoolVar(&flagVerify, "verify", false, "Validate server-path exists on Workato before saving")

	return cmd
}
