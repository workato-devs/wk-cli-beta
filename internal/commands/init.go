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
	var parentID *int
	for _, name := range parts {
		list, err := folders.List(cmd.Context(), parentID)
		if err != nil {
			return fmt.Errorf("verifying server path: %w", err)
		}
		found := false
		for _, f := range list {
			if strings.EqualFold(f.Name, name) {
				id := f.ID
				parentID = &id
				found = true
				break
			}
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
		flagWorkspace  string
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
				return fmt.Errorf("wk.toml already exists. Use 'wk link' to update workspace.")
			}

			name := flagName
			workspace := flagWorkspace

			if flagJSON {
				// Non-interactive: require --name and --workspace.
				if name == "" {
					return fmt.Errorf("--name is required in non-interactive (--json) mode")
				}
				if workspace == "" {
					return fmt.Errorf("--workspace is required in non-interactive (--json) mode")
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
				if workspace == "" {
					fmt.Print("Workspace profile: ")
					workspace, _ = reader.ReadString('\n')
					workspace = strings.TrimSpace(workspace)
					if workspace == "" {
						return fmt.Errorf("workspace profile is required")
					}
				}
			}

			cfg := &config.Config{
				Name:      name,
				Workspace: workspace,
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
				"status":    "initialized",
				"name":      name,
				"workspace": workspace,
				"path":      configPath,
			}

			if flagJSON {
				return rctx.Formatter.Format(cmd.OutOrStdout(), result)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Initialized wk project %q (workspace: %s) at %s\n", name, workspace, configPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&flagName, "name", "", "Project name")
	cmd.Flags().StringVar(&flagWorkspace, "workspace", "", "Workspace profile name")
	cmd.Flags().StringVar(&flagServerPath, "server-path", "", "Initial sync server path")
	cmd.Flags().StringVar(&flagLocalPath, "local-path", "", "Initial sync local path (defaults to server-path leaf)")
	cmd.Flags().BoolVar(&flagVerify, "verify", false, "Validate server-path exists on Workato before saving")

	return cmd
}
