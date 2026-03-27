package commands

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
)

func newAPICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Manage API Platform resources",
	}
	cmd.AddCommand(newAPICollectionsCmd())
	cmd.AddCommand(newAPIEndpointsCmd())
	return cmd
}

func newAPICollectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "collections",
		Aliases: []string{"collection"},
		Short:   "Manage API collections",
	}
	cmd.AddCommand(newAPICollectionsListCmd())
	cmd.AddCommand(newAPICollectionsCreateCmd())
	return cmd
}

func newAPICollectionsListCmd() *cobra.Command {
	var page, perPage int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API collections",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			opts := &api.PaginationOptions{Page: page, PerPage: perPage}
			collections, err := client.APICollections().List(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, collections)
			}

			headers := []string{"ID", "NAME", "HANDLE", "VERSION", "DESCRIPTION", "PROJECT ID"}
			var rows [][]string
			for _, c := range collections {
				rows = append(rows, []string{
					strconv.Itoa(c.ID),
					c.Name,
					c.Handle,
					c.Version,
					c.Description,
					strconv.Itoa(c.ProjectID),
				})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().IntVar(&page, "page", 0, "Page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newAPICollectionsCreateCmd() *cobra.Command {
	var name string
	var projectID int

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an API collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			collection, err := client.APICollections().Create(cmd.Context(), name, projectID)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, collection)
			}

			fmt.Fprintf(os.Stdout, "Created API collection %q (ID: %d)\n", collection.Name, collection.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Collection name")
	cmd.Flags().IntVar(&projectID, "project", 0, "Project ID")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func newAPIEndpointsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "endpoints",
		Aliases: []string{"endpoint"},
		Short:   "Manage API endpoints",
	}
	cmd.AddCommand(newAPIEndpointsListCmd())
	cmd.AddCommand(newAPIEndpointsEnableCmd())
	cmd.AddCommand(newAPIEndpointsDisableCmd())
	return cmd
}

func newAPIEndpointsListCmd() *cobra.Command {
	var collectionID, page, perPage int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API endpoints",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			var cid *int
			if cmd.Flags().Changed("collection") {
				cid = &collectionID
			}
			opts := &api.PaginationOptions{Page: page, PerPage: perPage}

			endpoints, err := client.APIEndpoints().List(cmd.Context(), cid, opts)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, endpoints)
			}

			headers := []string{"ID", "NAME", "METHOD", "PATH", "RECIPE ID", "COLLECTION ID", "ACTIVE"}
			var rows [][]string
			for _, e := range endpoints {
				active := "no"
				if e.Active {
					active = "yes"
				}
				rows = append(rows, []string{
					strconv.Itoa(e.ID),
					e.Name,
					e.Method,
					e.Path,
					strconv.Itoa(e.RecipeID),
					strconv.Itoa(e.APICollectionID),
					active,
				})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().IntVar(&collectionID, "collection", 0, "Filter by collection ID")
	cmd.Flags().IntVar(&page, "page", 0, "Page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newAPIEndpointsEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "Enable an API endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid endpoint ID: %s", args[0])
			}

			if err := client.APIEndpoints().Enable(cmd.Context(), id); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Endpoint %d enabled\n", id)
			return nil
		},
	}
}

func newAPIEndpointsDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "Disable an API endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid endpoint ID: %s", args[0])
			}

			if err := client.APIEndpoints().Disable(cmd.Context(), id); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Endpoint %d disabled\n", id)
			return nil
		},
	}
}
