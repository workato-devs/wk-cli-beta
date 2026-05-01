package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the wk CLI version",
		Example: `  wk version
  wk version --json`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			info := map[string]string{
				"version": versionStr,
				"commit":  commitStr,
				"date":    dateStr,
			}

			if flagJSON {
				return rctx.Formatter.Format(cmd.OutOrStdout(), info)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "wk version %s (commit: %s, built: %s)\n", versionStr, commitStr, dateStr)
			return nil
		},
	}
}
