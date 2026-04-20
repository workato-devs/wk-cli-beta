package commands

import "github.com/spf13/cobra"

// newSyncCmd is the top-level parent for sync-entry lifecycle commands.
// ADR-007 Decision 8 rationalizes this namespace — config verbs for sync
// entries (add/list/refresh/remove) sit alongside the transfer verbs
// (push/pull) rather than buried under `wk project sync`.
func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Manage [[sync]] entries in wk.toml",
		Long: `Sync entries describe the mapping between a Workato server folder and a
local directory inside the project container. Push and pull transfer
content; the commands here manage the entries themselves.

  wk sync add        Declare new entries from flags (--project / --projects-dir / --sync)
  wk sync list       Show all entries (folder_id as — when uncached)
  wk sync refresh    Reconcile cached folder_id values against the workspace
  wk sync remove     Remove an entry from wk.toml (optionally purge local files)`,
	}
	cmd.AddCommand(newSyncAddCmd())
	cmd.AddCommand(newSyncListCmd())
	cmd.AddCommand(newSyncRemoveCmd())
	return cmd
}
