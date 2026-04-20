package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
	"github.com/workato-devs/wk-cli-beta/internal/sync"
)

// unresolvedFolderID is the placeholder rendered in the default (human)
// table when a sync entry's folder_id has not yet been cached. Uncached
// rows are visible at a glance without needing a separate flag.
const unresolvedFolderID = "—"

// syncListRow is the structured form of a wk sync list row. Emitted under
// --json; also used to compute the table body so both output paths share
// the same source of truth.
type syncListRow struct {
	ServerPath   string     `json:"server_path"`
	LocalPath    string     `json:"local_path"`
	FolderID     int        `json:"folder_id,omitempty"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
}

// newSyncListCmd prints the [[sync]] entries in the current project.
// ADR-007 Decision 9: default columns are SERVER_PATH, LOCAL_PATH,
// FOLDER_ID (uncached → em-dash). --verbose (inherited from root)
// adds LAST_SYNCED_AT from .wk/ sidecar metadata without calling the
// API. --json emits a structured array, including last_synced_at when
// --verbose is also set.
func newSyncListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured sync entries",
		Long: `Print all [[sync]] entries in wk.toml. Entries whose folder_id has not
yet been cached render as "` + unresolvedFolderID + `" in the FOLDER_ID column — run
'wk sync refresh' to populate them without a push or pull.

--verbose adds a LAST_SYNCED_AT column computed from .wk/ sidecar metadata
(no API call). Works for both the default table and --json output; scripts
that pass --json --verbose get last_synced_at in the emitted objects.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			if rctx.Config == nil {
				return wkerrors.ErrNotInProject
			}

			rows := make([]syncListRow, 0, len(rctx.Config.Sync))
			for _, e := range rctx.Config.Sync {
				row := syncListRow{
					ServerPath: e.ServerPath,
					LocalPath:  e.LocalPath,
					FolderID:   e.FolderID,
				}
				if flagVerbose {
					row.LastSyncedAt = latestLastSynced(rctx.ProjectRoot, e.LocalPath)
				}
				rows = append(rows, row)
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, rows)
			}

			if len(rows) == 0 {
				if !rctx.Quiet {
					fmt.Fprintln(os.Stderr, "No [[sync]] entries configured.")
				}
				return nil
			}

			headers := []string{"SERVER_PATH", "LOCAL_PATH", "FOLDER_ID"}
			if flagVerbose {
				headers = append(headers, "LAST_SYNCED_AT")
			}
			tableRows := make([][]string, 0, len(rows))
			for _, r := range rows {
				folderID := unresolvedFolderID
				if r.FolderID != 0 {
					folderID = fmt.Sprintf("%d", r.FolderID)
				}
				row := []string{r.ServerPath, r.LocalPath, folderID}
				if flagVerbose {
					row = append(row, formatLastSynced(r.LastSyncedAt))
				}
				tableRows = append(tableRows, row)
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, tableRows)
		},
	}
	return cmd
}

// latestLastSynced scans the .wk/ sidecar tree for metas under localPath
// and returns the most-recent LastPulledAt timestamp. Returns nil when no
// metadata is present (entry has never been pulled or pushed). Any scan
// error is swallowed — --verbose is best-effort (no API, no failure
// path), and a missing/corrupt sidecar should not block the list.
func latestLastSynced(projectRoot, localPath string) *time.Time {
	metas, err := sync.FindMetaFiles(projectRoot, filepath.Join(projectRoot, localPath))
	if err != nil || len(metas) == 0 {
		return nil
	}
	var latest time.Time
	for _, m := range metas {
		if m.LastPulledAt.After(latest) {
			latest = m.LastPulledAt
		}
	}
	if latest.IsZero() {
		return nil
	}
	return &latest
}

func formatLastSynced(t *time.Time) string {
	if t == nil {
		return unresolvedFolderID
	}
	return t.Format(time.RFC3339)
}
