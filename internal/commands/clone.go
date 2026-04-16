package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/config"
	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
	"github.com/workato-devs/wk-cli-beta/internal/sync"
)

func newCloneCmd() *cobra.Command {
	var (
		flagPathPrefix string
		flagLocalPath  string
	)

	cmd := &cobra.Command{
		Use:   "clone <folder-name>",
		Short: "Clone a remote folder into a new local project",
		Long:  "Initialize a new wk project and pull assets from the specified remote folder.",
		Args: requireArgs(1, "folder name is required, e.g.: wk clone <folder-name>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			folderName := args[0]

			// Nesting guard: prevent cloning inside an existing project.
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}
			if projectRoot, err := config.FindProjectRoot(cwd); err == nil {
				return fmt.Errorf("%w at %s — run from outside the project directory", wkerrors.ErrNestedProject, projectRoot)
			}

			// Determine local directory
			localPath := flagLocalPath
			if localPath == "" {
				localPath = folderName
			}

			absPath, err := filepath.Abs(localPath)
			if err != nil {
				return fmt.Errorf("resolving path: %w", err)
			}

			// Create the directory
			if err := os.MkdirAll(absPath, 0755); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}

			// Build config — Name matches the actual directory, not the server folder.
			serverPath := folderName
			if flagPathPrefix != "" {
				serverPath = flagPathPrefix + "/" + folderName
			}

			cfg := &config.Config{
				Name: filepath.Base(absPath),
				Sync: []config.SyncEntry{
					{
						ServerPath: serverPath,
						LocalPath:  ".",
					},
				},
			}

			// Save wk.toml
			cfgPath := filepath.Join(absPath, config.ProjectFile)
			if err := config.Save(cfgPath, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			// Resolve API client
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			// Create sync engine and pull
			engine := sync.NewSyncEngine(absPath, cfg, client)
			entry := cfg.Sync[0]
			results, err := engine.Pull(entry, true) // force=true since fresh clone
			if err != nil {
				return fmt.Errorf("pulling assets: %w", err)
			}

			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Cloned %q into %s\n", folderName, absPath)

			if len(results) > 0 {
				headers := []string{"FILE", "ACTION"}
				var rows [][]string
				for _, r := range results {
					rows = append(rows, []string{r.FilePath, r.Action})
				}
				return rctx.Formatter.FormatList(os.Stdout, headers, rows)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&flagPathPrefix, "path-prefix", "", "Server-side path prefix for the folder")
	cmd.Flags().StringVar(&flagLocalPath, "local-path", "", "Local directory to clone into (default: folder name)")

	return cmd
}
