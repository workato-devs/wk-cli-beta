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

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage project-level configuration (sync entries, etc.)",
	}
	cmd.AddCommand(newProjectSyncCmd())
	return cmd
}

func newProjectSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Manage [[sync]] entries in wk.toml",
	}
	cmd.AddCommand(newProjectSyncListCmd())
	cmd.AddCommand(newProjectSyncAddCmd())
	cmd.AddCommand(newProjectSyncRemoveCmd())
	return cmd
}

func newProjectSyncListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured sync entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}
			if len(rctx.Config.Sync) == 0 {
				if !rctx.Quiet {
					fmt.Fprintln(os.Stderr, "No [[sync]] entries configured.")
				}
				return nil
			}
			headers := []string{"SERVER_PATH", "LOCAL_PATH", "FOLDER_ID"}
			rows := make([][]string, 0, len(rctx.Config.Sync))
			for _, e := range rctx.Config.Sync {
				folderID := ""
				if e.FolderID != 0 {
					folderID = fmt.Sprintf("%d", e.FolderID)
				}
				rows = append(rows, []string{e.ServerPath, e.LocalPath, folderID})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}
}

func newProjectSyncAddCmd() *cobra.Command {
	var flagVerify bool
	cmd := &cobra.Command{
		Use:   "add <server-path> [local-path]",
		Short: "Add a [[sync]] entry to wk.toml",
		Long: `Add a new [[sync]] entry that maps a Workato server-side folder to a local
directory. If local-path is omitted it defaults to the server-path's last
segment (e.g. "Recipes/Slack" -> "./Slack").

--verify calls the Workato API to confirm the server-path exists before
persisting the entry. Requires the project's bound profile to be usable.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}

			serverPath := strings.TrimSpace(args[0])
			if serverPath == "" {
				return fmt.Errorf("server-path cannot be empty")
			}
			localPath := ""
			if len(args) == 2 {
				localPath = strings.TrimSpace(args[1])
			}
			if localPath == "" {
				localPath = defaultLocalPathForServerPath(serverPath)
			}

			for _, e := range rctx.Config.Sync {
				if e.ServerPath == serverPath && e.LocalPath == localPath {
					return fmt.Errorf("sync entry already exists: server_path=%q local_path=%q", serverPath, localPath)
				}
			}

			if flagVerify {
				if rctx.Config.Profile == "" {
					return fmt.Errorf("--verify requires the project to have a bound profile (cfg.Profile)")
				}
				client, verr := resolveVerifyClient(cmd, rctx.Config.Profile)
				if verr != nil {
					return fmt.Errorf("--verify requires auth: %w", verr)
				}
				if _, verr := verifyServerPath(cmd, client, serverPath); verr != nil {
					return verr
				}
			}

			rctx.Config.Sync = append(rctx.Config.Sync, config.SyncEntry{
				ServerPath: serverPath,
				LocalPath:  localPath,
			})
			if err := config.Save(config.ProjectConfigPath(rctx.ProjectRoot), rctx.Config); err != nil {
				return fmt.Errorf("saving wk.toml: %w", err)
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, map[string]string{
					"status":      "added",
					"server_path": serverPath,
					"local_path":  localPath,
				})
			}
			fmt.Fprintf(os.Stdout, "Added sync entry: %s -> %s\n", serverPath, localPath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagVerify, "verify", false, "Validate server-path exists on Workato before saving")
	return cmd
}

func newProjectSyncRemoveCmd() *cobra.Command {
	var flagPurge bool
	cmd := &cobra.Command{
		Use:   "remove <server-path>",
		Short: "Remove a [[sync]] entry from wk.toml",
		Long: `Remove the [[sync]] entry matching server-path. By default this only
rewrites wk.toml — the local directory and its .wk/ meta mirror are left
alone. Pass --purge to also remove the local directory and its metas.`,
		Args: requireArgs(1, "server-path is required, e.g.: wk project sync remove <server-path>"),
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

// parseSyncFlag parses a --sync value of the form "SERVER_PATH" or
// "SERVER_PATH:LOCAL_PATH" into a SyncEntry. Server paths are Workato
// forward-slash paths and never contain colons, so the first colon is the
// separator. Local path defaults to defaultLocalPathForServerPath when
// omitted.
func parseSyncFlag(raw string) (config.SyncEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return config.SyncEntry{}, fmt.Errorf("--sync value cannot be empty")
	}
	var server, local string
	if i := strings.IndexByte(raw, ':'); i >= 0 {
		server = strings.TrimSpace(raw[:i])
		local = strings.TrimSpace(raw[i+1:])
	} else {
		server = raw
	}
	if server == "" {
		return config.SyncEntry{}, fmt.Errorf("--sync %q missing server_path", raw)
	}
	if local == "" {
		local = defaultLocalPathForServerPath(server)
	}
	return config.SyncEntry{ServerPath: server, LocalPath: local}, nil
}

// defaultLocalPathForServerPath picks a sensible local dir for a server
// path when none was provided: the last /-separated segment, prefixed with
// "./", with "All projects" stripped if present.
func defaultLocalPathForServerPath(serverPath string) string {
	trimmed := strings.Trim(serverPath, "/")
	if trimmed == "" {
		return "."
	}
	parts := strings.Split(trimmed, "/")
	if strings.EqualFold(parts[0], "All projects") {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "."
	}
	return "./" + parts[len(parts)-1]
}
