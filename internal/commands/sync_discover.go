package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

// discoverRow is the structured form of a wk sync discover row. Emitted
// under --json; also used to compute the table body so both output paths
// share the same source of truth.
type discoverRow struct {
	Folder    string `json:"folder"`
	Status    string `json:"status"`
	LocalPath string `json:"local_path,omitempty"`
}

// newSyncDiscoverCmd queries the workspace for top-level folders and
// compares them against local [[sync]] entries in wk.toml. Reports which
// remote folders are mapped (with their local path) and which are
// unmapped — surfacing server-side folders that status/pull cannot see
// because they have no sync config entry (issue #61).
func newSyncDiscoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "List server-side folders not yet in sync config",
		Long: `Query the workspace for top-level folders and compare against local
[[sync]] entries in wk.toml. Reports which remote folders are mapped and
which are unmapped.

Mapped folders show the local_path they sync to. Unmapped folders are
invisible to 'wk status' and 'wk pull' — use 'wk sync add' to declare
entries for them.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}

			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			folders, err := client.Folders().List(ctx, nil)
			if err != nil {
				return fmt.Errorf("listing workspace folders: %w", err)
			}

			// Build set of configured server paths (case-insensitive).
			mapped := make(map[string]string, len(rctx.Config.Sync))
			for _, entry := range rctx.Config.Sync {
				mapped[strings.ToLower(entry.ServerPath)] = entry.LocalPath
			}

			rows := make([]discoverRow, 0, len(folders))
			for _, f := range folders {
				row := discoverRow{Folder: f.Name}
				if localPath, ok := mapped[strings.ToLower(f.Name)]; ok {
					row.Status = "mapped"
					row.LocalPath = localPath
				} else {
					row.Status = "unmapped"
				}
				rows = append(rows, row)
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, rows)
			}

			if len(rows) == 0 {
				if !rctx.Quiet {
					fmt.Fprintln(os.Stderr, "No folders found in workspace.")
				}
				return nil
			}

			headers := []string{"FOLDER", "STATUS"}
			tableRows := make([][]string, 0, len(rows))
			for _, r := range rows {
				status := r.Status
				if r.LocalPath != "" {
					status = "mapped → " + r.LocalPath
				}
				tableRows = append(tableRows, []string{r.Folder, status})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, tableRows)
		},
	}
	return cmd
}
