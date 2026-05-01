package commands

import (
	"os"

	"github.com/spf13/cobra"
)

func newConnectorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connectors",
		Aliases: []string{"connector"},
		Short:   "Manage custom SDK connectors (use 'wk connections' for workspace connections)",
	}
	cmd.AddCommand(newConnectorsListCmd())
	return cmd
}

func newConnectorsListCmd() *cobra.Command {
	var search string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List custom SDK connectors",
		Example: `  wk connectors list
  wk connectors list --search salesforce --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			connectors, err := client.Connectors().List(cmd.Context(), search)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, connectors)
			}

			headers := []string{"NAME", "TITLE", "DESCRIPTION"}
			var rows [][]string
			for _, c := range connectors {
				rows = append(rows, []string{c.Name, c.Title, c.Description})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&search, "search", "", "Search custom SDK connectors")
	return cmd
}
