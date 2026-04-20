package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/config"
	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

// newSyncAddCmd adds one or more [[sync]] entries to wk.toml using the
// unified flag surface shared with `wk init` (ADR-007 Decisions 1-3, 8).
//
// Dedup semantics: exact (server_path, local_path) duplicates among the
// new flags error out (typo-catcher from PR B). Exact-matches against
// entries already in wk.toml are silently skipped — Decision 8 point 7:
// zero-add is a valid no-op, not an error.
func newSyncAddCmd() *cobra.Command {
	var syncFlags SyncEntryFlags
	var flagVerify bool

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add one or more [[sync]] entries to wk.toml",
		Long: `Declare new sync entries with the same flag surface as 'wk init':

  --project <name>            name-based, repeatable
  --projects-dir <relpath>    discovery (alone) or local_path prefix (with --project)
  --sync SERVER[:LOCAL]       fine-grained single mapping

Pass --verify to validate every declared server-path against the workspace
and cache the resolved folder_id in wk.toml. Entries that already exist
(same server_path AND local_path) are silently skipped; duplicate flags
within one invocation are flagged as errors.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}

			requested, err := AssembleSyncEntries(&syncFlags, rctx.ProjectRoot)
			if err != nil {
				return err
			}
			if len(requested) == 0 {
				return fmt.Errorf("no entries to add — pass --project, --projects-dir, or --sync")
			}

			if flagVerify {
				if rctx.Config.Profile == "" {
					return fmt.Errorf("--verify requires the project to have a bound profile (cfg.Profile)")
				}
				client, verr := resolveVerifyClient(cmd, rctx.Config.Profile)
				if verr != nil {
					return fmt.Errorf("--verify requires auth: %w", verr)
				}
				for i := range requested {
					leaf, err := verifyServerPath(cmd, client, requested[i].ServerPath)
					if err != nil {
						return err
					}
					if leaf != nil {
						requested[i].FolderID = leaf.ID
						requested[i].ProjectID = leaf.ProjectID
					}
				}
			}

			var added []config.SyncEntry
			for _, newEntry := range requested {
				dup := false
				for _, existing := range rctx.Config.Sync {
					if existing.ServerPath == newEntry.ServerPath && existing.LocalPath == newEntry.LocalPath {
						dup = true
						break
					}
				}
				if !dup {
					added = append(added, newEntry)
				}
			}

			rctx.Config.Sync = append(rctx.Config.Sync, added...)
			if err := config.Save(config.ProjectConfigPath(rctx.ProjectRoot), rctx.Config); err != nil {
				return fmt.Errorf("saving wk.toml: %w", err)
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, map[string]any{
					"status":      "added",
					"added_count": len(added),
					"entries":     added,
				})
			}
			if len(added) == 0 {
				fmt.Fprintf(os.Stderr, "No new sync entries added — all declared entries already exist in wk.toml.\n")
				return nil
			}
			fmt.Fprintf(os.Stdout, "Added %d sync entr%s:\n", len(added), pluralY(len(added)))
			for _, e := range added {
				fmt.Fprintf(os.Stdout, "  %s -> %s\n", e.ServerPath, e.LocalPath)
			}
			return nil
		},
	}
	BindSyncEntryFlags(cmd, &syncFlags)
	cmd.Flags().BoolVar(&flagVerify, "verify", false,
		"Validate every declared server-path against Workato and cache resolved IDs in wk.toml")
	return cmd
}
