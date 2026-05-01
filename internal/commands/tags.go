package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/output"
)

func newTagsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tags",
		Aliases: []string{"tag"},
		Short:   "Manage Workato tags",
	}
	cmd.AddCommand(newTagsListCmd())
	cmd.AddCommand(newTagsCreateCmd())
	cmd.AddCommand(newTagsUpdateCmd())
	cmd.AddCommand(newTagsDeleteCmd())
	cmd.AddCommand(newTagsApplyCmd())
	cmd.AddCommand(newTagsRemoveCmd())
	return cmd
}

func newTagsListCmd() *cobra.Command {
	var search string
	var page, perPage int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags",
		Example: `  wk tags list
  wk tags list --search "deploy" --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			opts := &api.TagListOptions{Search: search, Page: page, PerPage: perPage}
			tags, err := client.Tags().List(cmd.Context(), opts)
			if err != nil {
				return err
			}

			headers := []string{"HANDLE", "TITLE", "DESCRIPTION", "COLOR"}
			var rows [][]string
			for _, t := range tags {
				rows = append(rows, []string{t.Handle, t.Title, t.Description, t.Color})
			}
			meta := output.PageMeta{Page: page, PerPage: perPage, HasNext: perPage > 0 && len(tags) == perPage}
			return rctx.Formatter.FormatPage(os.Stdout, headers, rows, meta)
		},
	}

	cmd.Flags().StringVar(&search, "search", "", "Search tags by name")
	cmd.Flags().IntVar(&page, "page", 0, "Page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newTagsCreateCmd() *cobra.Command {
	var description, color string

	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a tag",
		Example: `  wk tags create "production" --color blue
  wk tags create "staging" --description "Pre-prod environment" --json`,
		Args:  requireArgs(1, "tag title is required, e.g.: wk tags create <title>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			tag, err := client.Tags().Create(cmd.Context(), args[0], description, color)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, tag)
			}

			fmt.Fprintf(os.Stderr, "Created tag %q (handle: %s)\n", tag.Title, tag.Handle)
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Tag description")
	cmd.Flags().StringVar(&color, "color", "", "Tag color")
	return cmd
}

func newTagsUpdateCmd() *cobra.Command {
	var title, description, color string

	cmd := &cobra.Command{
		Use:   "update <handle>",
		Short: "Update a tag",
		Example: `  wk tags update production --title "Production" --color green`,
		Args:  requireArgs(1, "tag handle is required, e.g.: wk tags update <handle>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			opts := &api.TagUpdateOptions{}
			if cmd.Flags().Changed("title") {
				opts.Title = &title
			}
			if cmd.Flags().Changed("description") {
				opts.Description = &description
			}
			if cmd.Flags().Changed("color") {
				opts.Color = &color
			}

			tag, err := client.Tags().Update(cmd.Context(), args[0], opts)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, tag)
			}

			fmt.Fprintf(os.Stderr, "Tag %q updated\n", tag.Handle)
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "New title")
	cmd.Flags().StringVar(&description, "description", "", "New description")
	cmd.Flags().StringVar(&color, "color", "", "New color")
	return cmd
}

func newTagsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <handle>",
		Short:   "Delete a tag",
		Example: `  wk tags delete staging`,
		Args:  requireArgs(1, "tag handle is required, e.g.: wk tags delete <handle>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			if err := client.Tags().Delete(cmd.Context(), args[0]); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Tag %q deleted\n", args[0])
			return nil
		},
	}
}

func newTagsApplyCmd() *cobra.Command {
	var targets []string

	cmd := &cobra.Command{
		Use:   "apply <handle>",
		Short: "Apply a tag to recipes or connections",
		Example: `  wk tags apply production --to recipe:123
  wk tags apply production --to recipe:123,connection:456`,
		Args:  requireArgs(1, "tag handle is required, e.g.: wk tags apply <handle> --to recipe:123"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			recipeIDs, connectionIDs, err := parseTargets(targets)
			if err != nil {
				return err
			}

			if err := client.Tags().Assign(cmd.Context(), []string{args[0]}, nil, recipeIDs, connectionIDs); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Tag %q applied\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&targets, "to", nil, "Target (e.g., recipe:123, connection:456)")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func newTagsRemoveCmd() *cobra.Command {
	var targets []string

	cmd := &cobra.Command{
		Use:   "remove <handle>",
		Short: "Remove a tag from recipes or connections",
		Example: `  wk tags remove staging --from recipe:123`,
		Args:  requireArgs(1, "tag handle is required, e.g.: wk tags remove <handle> --from recipe:123"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			recipeIDs, connectionIDs, err := parseTargets(targets)
			if err != nil {
				return err
			}

			if err := client.Tags().Assign(cmd.Context(), nil, []string{args[0]}, recipeIDs, connectionIDs); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Tag %q removed\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&targets, "from", nil, "Target (e.g., recipe:123, connection:456)")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func parseTargets(targets []string) (recipeIDs, connectionIDs []int, err error) {
	for _, t := range targets {
		parts := strings.SplitN(t, ":", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid target %q: expected type:id (e.g., recipe:123)", t)
		}
		id, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, nil, fmt.Errorf("invalid ID in target %q: %w", t, err)
		}
		switch parts[0] {
		case "recipe":
			recipeIDs = append(recipeIDs, id)
		case "connection":
			connectionIDs = append(connectionIDs, id)
		default:
			return nil, nil, fmt.Errorf("unknown target type %q in %q", parts[0], t)
		}
	}
	return recipeIDs, connectionIDs, nil
}
