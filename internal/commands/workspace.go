package commands

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
)

func newWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Workspace management commands",
	}
	cmd.AddCommand(newWorkspaceInfoCmd())
	cmd.AddCommand(newWorkspaceUsersCmd())
	cmd.AddCommand(newWorkspaceAuditLogCmd())
	return cmd
}

func newWorkspaceInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show current workspace info",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			user, err := client.Workspace().GetCurrentUser(cmd.Context())
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, user)
			}

			fmt.Fprintf(os.Stdout, "ID:    %d\n", user.ID)
			fmt.Fprintf(os.Stdout, "Name:  %s\n", user.Name)
			fmt.Fprintf(os.Stdout, "Email: %s\n", user.Email)
			return nil
		},
	}
}

func newWorkspaceUsersCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "users",
		Short: "List workspace members",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			members, err := client.Workspace().ListMembers(cmd.Context(), email)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, members)
			}

			headers := []string{"ID", "NAME", "EMAIL"}
			var rows [][]string
			for _, m := range members {
				rows = append(rows, []string{
					strconv.Itoa(m.ID),
					m.Name,
					m.Email,
				})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Filter by email")
	return cmd
}

func newWorkspaceAuditLogCmd() *cobra.Command {
	var since, until, action string

	cmd := &cobra.Command{
		Use:   "audit-log",
		Short: "View workspace audit log",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			opts := &api.AuditLogOptions{
				Since:  since,
				Until:  until,
				Action: action,
			}

			entries, err := client.Workspace().GetAuditLogs(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, entries)
			}

			headers := []string{"ID", "EVENT TYPE", "USER", "TIMESTAMP"}
			var rows [][]string
			for _, e := range entries {
				rows = append(rows, []string{
					strconv.Itoa(e.ID),
					e.EventType,
					e.User.Email,
					e.Timestamp,
				})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Start date (RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End date (RFC3339)")
	cmd.Flags().StringVar(&action, "action", "", "Filter by event type")
	return cmd
}
