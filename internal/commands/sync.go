package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/config"
	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
	"github.com/workato-devs/wk-cli-beta/internal/plugin"
	"github.com/workato-devs/wk-cli-beta/internal/sync"
)

// resolveSyncEntries returns the sync entries to operate on.
// If folder is empty, returns all entries. Otherwise returns the matching entry.
func resolveSyncEntries(cfg *config.Config, folder string) ([]config.SyncEntry, error) {
	if len(cfg.Sync) == 0 {
		return nil, wkerrors.ErrNoSyncEntries
	}
	if folder == "" {
		return cfg.Sync, nil
	}
	for _, entry := range cfg.Sync {
		if entry.ServerPath == folder || entry.LocalPath == folder {
			return []config.SyncEntry{entry}, nil
		}
	}
	return nil, fmt.Errorf("no sync entry matching %q", folder)
}

func newPullCmd() *cobra.Command {
	var (
		flagFolder string
		flagForce  bool
	)

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull remote assets to local project",
		Long:  "Download assets from the Workato workspace into the local project directory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}

			entries, err := resolveSyncEntries(rctx.Config, flagFolder)
			if err != nil {
				return err
			}

			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			engine := sync.NewSyncEngine(rctx.ProjectRoot, rctx.Config, client)
			var allResults []sync.PullResult
			for _, entry := range entries {
				results, err := engine.Pull(entry, flagForce)
				if err != nil {
					return err
				}
				allResults = append(allResults, results...)
			}

			if len(allResults) == 0 {
				if !rctx.Quiet {
					fmt.Fprintln(os.Stderr, "No assets to pull.")
				}
				return nil
			}

			headers := []string{"FILE", "ACTION"}
			var rows [][]string
			for _, r := range allResults {
				rows = append(rows, []string{r.FilePath, r.Action})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&flagFolder, "folder", "", "Sync entry filter (server_path or local_path)")
	cmd.Flags().BoolVar(&flagForce, "force", false, "Overwrite local modifications")

	return cmd
}

func newPushCmd() *cobra.Command {
	var (
		flagFolder        string
		flagDryRun        bool
		flagPreserveState bool
		flagSkipHooks     bool
	)

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push local changes to remote workspace",
		Long:  "Upload modified local assets to the Workato workspace.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}

			entries, err := resolveSyncEntries(rctx.Config, flagFolder)
			if err != nil {
				return err
			}

			// Run pre-push hooks first (local-only, no API client needed).
			if !flagSkipHooks && !flagDryRun && rctx.PluginRegistry != nil {
				for _, entry := range entries {
					if err := runPrePushHooks(rctx, entry); err != nil {
						return err
					}
				}
			}

			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			engine := sync.NewSyncEngine(rctx.ProjectRoot, rctx.Config, client)
			var allResults []sync.PushResult
			for _, entry := range entries {
				results, err := engine.Push(entry, flagDryRun, flagPreserveState)
				if err != nil {
					return err
				}
				allResults = append(allResults, results...)
			}

			if len(allResults) == 0 {
				if !rctx.Quiet {
					fmt.Fprintln(os.Stderr, "No changes to push.")
				}
				return nil
			}

			if flagDryRun && !rctx.Quiet {
				fmt.Fprintln(os.Stderr, "Dry run -- no changes were pushed.")
			}

			headers := []string{"FILE", "ACTION"}
			var rows [][]string
			for _, r := range allResults {
				rows = append(rows, []string{r.FilePath, r.Action})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&flagFolder, "folder", "", "Sync entry filter (server_path or local_path)")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would be pushed without uploading")
	cmd.Flags().BoolVar(&flagPreserveState, "preserve-state", true, "Preserve recipe active state on import")
	cmd.Flags().BoolVar(&flagSkipHooks, "skip-hooks", false, "Skip plugin pre-push hooks")

	return cmd
}

// runPrePushHooks runs all registered pre-push hooks and blocks push if any fail.
func runPrePushHooks(rctx *RunContext, entry config.SyncEntry) error {
	// Status() is local-only — no API client needed.
	statusEngine := sync.NewSyncEngine(rctx.ProjectRoot, rctx.Config, nil)
	statuses, err := statusEngine.Status(entry)
	if err != nil {
		// Non-fatal: warn and continue.
		fmt.Fprintf(os.Stderr, "Warning: could not compute file status for hooks: %v\n", err)
		return nil
	}

	hookFiles := make([]plugin.HookFile, len(statuses))
	for i, s := range statuses {
		hookFiles[i] = plugin.HookFile{
			Path:       filepath.Join(entry.LocalPath, s.FilePath),
			Status:     string(s.Status),
			ServerPath: s.ServerPath,
		}
	}

	params := plugin.HookParams{
		ProjectRoot: rctx.ProjectRoot,
		Files:       hookFiles,
	}

	results, err := plugin.RunPrePushHook(rctx.PluginRegistry, params)
	if err != nil {
		// Fail-open: warn and continue.
		fmt.Fprintf(os.Stderr, "Warning: pre-push hook error: %v\n", err)
		return nil
	}
	if results == nil {
		return nil
	}

	// Print any diagnostics.
	plugin.FormatHookResults(os.Stderr, results, flagJSON)

	// Block push if any hook reported failure.
	for _, r := range results {
		if !r.Passed {
			return fmt.Errorf("pre-push hook %q failed — push blocked (use --skip-hooks to bypass)", r.PluginName)
		}
	}

	return nil
}

func newStatusCmd() *cobra.Command {
	var flagFolder string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync status of the current project",
		Long:  "Show which local files are new, modified, or deleted compared to the last pull.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}

			entries, err := resolveSyncEntries(rctx.Config, flagFolder)
			if err != nil {
				return err
			}

			// Status is local-only; no API client needed.
			engine := sync.NewSyncEngine(rctx.ProjectRoot, rctx.Config, nil)
			var allStatuses []sync.AssetStatus
			for _, entry := range entries {
				statuses, err := engine.Status(entry)
				if err != nil {
					return err
				}
				allStatuses = append(allStatuses, statuses...)
			}

			if len(allStatuses) == 0 {
				if !rctx.Quiet {
					fmt.Fprintln(os.Stderr, "No synced assets found.")
				}
				return nil
			}

			headers := []string{"FILE", "STATUS", "SERVER PATH"}
			var rows [][]string
			for _, s := range allStatuses {
				rows = append(rows, []string{s.FilePath, string(s.Status), s.ServerPath})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&flagFolder, "folder", "", "Sync entry filter (server_path or local_path)")

	return cmd
}

func newDiffCmd() *cobra.Command {
	var flagFolder string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show differences between local and remote",
		Long:  "Compare local assets against the remote workspace to show what has changed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}

			entries, err := resolveSyncEntries(rctx.Config, flagFolder)
			if err != nil {
				return err
			}

			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			engine := sync.NewSyncEngine(rctx.ProjectRoot, rctx.Config, client)
			var allDiffs []sync.DiffEntry
			for _, entry := range entries {
				diffs, err := engine.Diff(entry)
				if err != nil {
					return err
				}
				allDiffs = append(allDiffs, diffs...)
			}

			if len(allDiffs) == 0 {
				if !rctx.Quiet {
					fmt.Fprintln(os.Stderr, "No differences found.")
				}
				return nil
			}

			headers := []string{"PATH", "STATUS", "LOCAL HASH", "REMOTE HASH"}
			var rows [][]string
			for _, d := range allDiffs {
				localHash := truncateHash(d.LocalHash)
				remoteHash := truncateHash(d.RemoteHash)
				rows = append(rows, []string{d.Path, string(d.Type), localHash, remoteHash})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&flagFolder, "folder", "", "Sync entry filter (server_path or local_path)")

	return cmd
}

// truncateHash shortens a SHA256 hash for display (first 8 chars).
func truncateHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}
