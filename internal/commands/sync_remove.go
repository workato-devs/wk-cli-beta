package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/config"
	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

// newSyncRemoveCmd removes a [[sync]] entry from wk.toml, optionally
// also deleting the local directory and its .wk/ meta mirror under
// --purge. Never touches the server-side folder (ADR-007 Decision 10).
func newSyncRemoveCmd() *cobra.Command {
	var flagPurge bool

	cmd := &cobra.Command{
		Use:   "remove <server-path>",
		Short: "Remove a [[sync]] entry from wk.toml",
		Long: `Remove the [[sync]] entry matching <server-path>. By default this only
rewrites wk.toml — the local directory and its .wk/ meta mirror are left
alone. Pass --purge to also remove the local directory and its metas.

Never touches the server-side folder. Use 'wk folders delete' for that.`,
		Args: requireArgs(1, "server-path is required, e.g.: wk sync remove <server-path>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}
			serverPath := strings.TrimSpace(args[0])

			var kept []config.SyncEntry
			var removed []config.SyncEntry
			for _, e := range rctx.Config.Sync {
				if e.ServerPath == serverPath {
					removed = append(removed, e)
					continue
				}
				kept = append(kept, e)
			}
			if len(removed) == 0 {
				return fmt.Errorf("no sync entry with server_path=%q", serverPath)
			}
			rctx.Config.Sync = kept
			if err := config.Save(config.ProjectConfigPath(rctx.ProjectRoot), rctx.Config); err != nil {
				return fmt.Errorf("saving wk.toml: %w", err)
			}

			var purged []string
			if flagPurge {
				for _, e := range removed {
					if e.LocalPath == "" || e.LocalPath == "." {
						// Refuse to blanket-wipe the project root.
						fmt.Fprintf(os.Stderr, "warning: refusing to --purge local_path=%q (would remove project root or current dir); remove manually\n", e.LocalPath)
						continue
					}
					localAbs := filepath.Join(rctx.ProjectRoot, e.LocalPath)
					metaAbs := filepath.Join(rctx.ProjectRoot, config.ProjectDir, e.LocalPath)
					if err := os.RemoveAll(localAbs); err != nil {
						return fmt.Errorf("purging local dir %s: %w", localAbs, err)
					}
					if err := os.RemoveAll(metaAbs); err != nil {
						return fmt.Errorf("purging meta dir %s: %w", metaAbs, err)
					}
					purged = append(purged, e.LocalPath)
				}
			}

			if flagJSON {
				result := map[string]any{
					"status":      "removed",
					"server_path": serverPath,
				}
				if flagPurge {
					result["purged"] = purged
				}
				return rctx.Formatter.Format(os.Stdout, result)
			}
			fmt.Fprintf(os.Stdout, "Removed sync entry: %s\n", serverPath)
			for _, p := range purged {
				fmt.Fprintf(os.Stdout, "Purged local dir + metas: %s\n", p)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagPurge, "purge", false, "Also delete the local directory and its .wk/ meta mirror")
	return cmd
}
