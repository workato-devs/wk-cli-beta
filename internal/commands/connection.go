package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/output"
)

func newConnectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connections",
		Aliases: []string{"connection", "conn"},
		Short:   "Manage Workato connections",
	}
	cmd.AddCommand(newConnectionsListCmd())
	cmd.AddCommand(newConnectionsGetCmd())
	cmd.AddCommand(newConnectionsCreateCmd())
	cmd.AddCommand(newConnectionsUpdateCmd())
	cmd.AddCommand(newConnectionsDeleteCmd())
	cmd.AddCommand(newConnectionsDisconnectCmd())
	return cmd
}

func newConnectionsListCmd() *cobra.Command {
	var folderID int
	var application string
	var page, perPage int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List connections",
		Example: `  wk connections list
  wk connections list --folder 123 --application salesforce --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			opts := &api.ConnectionListOptions{
				Page:    page,
				PerPage: perPage,
			}
			if cmd.Flags().Changed("folder") {
				opts.FolderID = &folderID
			}

			conns, err := client.Connections().List(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if application != "" {
				var filtered []api.Connection
				for _, c := range conns {
					if strings.Contains(strings.ToLower(c.Application), strings.ToLower(application)) {
						filtered = append(filtered, c)
					}
				}
				conns = filtered
			}

			headers := []string{"ID", "NAME", "APPLICATION", "FOLDER", "STATUS"}
			var rows [][]string
			for _, c := range conns {
				status := "not connected"
				if c.AuthorizationStatus != nil && *c.AuthorizationStatus == "success" {
					status = "connected"
				} else if c.AuthorizationStatus != nil && *c.AuthorizationStatus != "" {
					status = *c.AuthorizationStatus
				}
				rows = append(rows, []string{
					strconv.Itoa(c.ID),
					c.Name,
					c.Application,
					strconv.Itoa(c.FolderID),
					status,
				})
			}
			meta := output.PageMeta{Page: page, PerPage: perPage, HasNext: perPage > 0 && len(conns) == perPage}
			return rctx.Formatter.FormatPage(os.Stdout, conns, headers, rows, meta)
		},
	}

	cmd.Flags().IntVar(&folderID, "folder", 0, "Filter by folder ID")
	cmd.Flags().StringVar(&application, "application", "", "Filter by application name")
	cmd.Flags().IntVar(&page, "page", 0, "Page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newConnectionsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get connection details",
		Example: `  wk connections get 456
  wk connections get 456 --json`,
		Args:  requireArgs(1, "connection ID is required, e.g.: wk connections get <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid connection ID: %s", args[0])
			}

			conn, err := client.Connections().Get(cmd.Context(), id)
			if err != nil {
				return err
			}

			if !flagJSON {
				status := "not connected"
				if conn.AuthorizationStatus != nil && *conn.AuthorizationStatus == "success" {
					status = "connected"
				} else if conn.AuthorizationStatus != nil && *conn.AuthorizationStatus != "" {
					status = *conn.AuthorizationStatus
				}
				fmt.Fprintf(os.Stdout, "ID:          %d\n", conn.ID)
				fmt.Fprintf(os.Stdout, "Name:        %s\n", conn.Name)
				fmt.Fprintf(os.Stdout, "Application: %s\n", conn.Application)
				fmt.Fprintf(os.Stdout, "Folder ID:   %d\n", conn.FolderID)
				fmt.Fprintf(os.Stdout, "Status:      %s\n", status)
				fmt.Fprintf(os.Stdout, "Updated:     %s\n", conn.UpdatedAt.Format(time.RFC3339))
				return nil
			}
			return rctx.Formatter.Format(os.Stdout, conn)
		},
	}
}

func newConnectionsCreateCmd() *cobra.Command {
	var name, provider string
	var folderID int

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a connection",
		Example: `  wk connections create --name "My Salesforce" --provider salesforce
  wk connections create --name "My DB" --provider postgresql --folder 123 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			var fid *int
			if cmd.Flags().Changed("folder") {
				fid = &folderID
			}

			conn, err := client.Connections().Create(cmd.Context(), name, provider, fid)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, conn)
			}

			fmt.Fprintf(os.Stderr, "Created connection %q (ID: %d)\n", conn.Name, conn.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Connection name")
	cmd.Flags().StringVar(&provider, "provider", "", "Connection provider/application")
	cmd.Flags().IntVar(&folderID, "folder", 0, "Folder ID")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("provider")
	return cmd
}

func newConnectionsUpdateCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a connection",
		Example: `  wk connections update 456 --name "Renamed Connection"`,
		Args:  requireArgs(1, "connection ID is required, e.g.: wk connections update <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid connection ID: %s", args[0])
			}

			conn, err := client.Connections().Update(cmd.Context(), id, name)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, conn)
			}

			fmt.Fprintf(os.Stderr, "Connection %d updated\n", conn.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "New connection name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newConnectionsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete a connection",
		Example: `  wk connections delete 456`,
		Args:  requireArgs(1, "connection ID is required, e.g.: wk connections delete <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid connection ID: %s", args[0])
			}

			if err := client.Connections().Delete(cmd.Context(), id); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Connection %d deleted\n", id)
			return nil
		},
	}
}

func newConnectionsDisconnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "disconnect <id>",
		Short:   "Disconnect a connection",
		Example: `  wk connections disconnect 456`,
		Args:  requireArgs(1, "connection ID is required, e.g.: wk connections disconnect <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid connection ID: %s", args[0])
			}

			if err := client.Connections().Disconnect(cmd.Context(), id); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Connection %d disconnected\n", id)
			return nil
		},
	}
}

