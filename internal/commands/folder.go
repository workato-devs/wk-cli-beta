package commands

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
)

func newFoldersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "folders",
		Aliases: []string{"folder"},
		Short:   "Manage Workato folders",
	}
	cmd.AddCommand(newFoldersListCmd())
	cmd.AddCommand(newFoldersCreateCmd())
	cmd.AddCommand(newFoldersDeleteCmd())
	return cmd
}

func newFoldersListCmd() *cobra.Command {
	var parentID int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List folders",
		Example: `  wk folders list
  wk folders list --parent 123 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			var pid *int
			if cmd.Flags().Changed("parent") {
				pid = &parentID
			}

			folders, err := client.Folders().List(cmd.Context(), pid)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, folders)
			}

			headers := []string{"ID", "NAME", "PARENT ID"}
			var rows [][]string
			for _, f := range folders {
				parent := ""
				if f.ParentID != nil {
					parent = strconv.Itoa(*f.ParentID)
				}
				rows = append(rows, []string{
					strconv.Itoa(f.ID),
					f.Name,
					parent,
				})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().IntVar(&parentID, "parent", 0, "Parent folder ID")
	return cmd
}

func newFoldersCreateCmd() *cobra.Command {
	var parentID int

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a folder",
		Example: `  wk folders create "Marketing Recipes"
  wk folders create "Subfolder" --parent 123 --json`,
		Args:  requireArgs(1, "folder name is required, e.g.: wk folders create <name>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			var pid *int
			if cmd.Flags().Changed("parent") {
				pid = &parentID
			}

			folder, err := client.Folders().Create(cmd.Context(), args[0], pid)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, folder)
			}

			fmt.Fprintf(os.Stderr, "Created folder %q (ID: %d)\n", folder.Name, folder.ID)
			return nil
		},
	}

	cmd.Flags().IntVar(&parentID, "parent", 0, "Parent folder ID")
	return cmd
}

func newFoldersDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a folder or project",
		Example: `  wk folders delete 123`,
		Long: `Delete a top-level Workato project or a nested folder. The Workato
API uses separate endpoints — DELETE /projects/{id} for projects
(is_project=true), DELETE /folders/{id} for plain folders. This
command lists top-level folders first and routes to the correct
endpoint based on the target's is_project flag.`,
		Args: requireArgs(1, "folder ID is required, e.g.: wk folders delete <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid folder ID: %s", args[0])
			}

			// Projects are always top-level; a single list call at the
			// workspace root resolves both is_project and project_id. If
			// the target isn't in the root list it's a nested folder,
			// which always routes to DELETE /folders/{id}.
			topLevel, err := client.Folders().List(cmd.Context(), nil)
			if err != nil {
				return fmt.Errorf("listing top-level folders to determine type: %w", err)
			}
			var match *api.Folder
			for i, f := range topLevel {
				if f.ID == id {
					match = &topLevel[i]
					break
				}
			}

			if match != nil && match.IsProject {
				// DELETE /projects/{project_id} — not folder_id. The
				// project_id is a separate identifier returned on the
				// folder list response when is_project=true.
				if match.ProjectID == 0 {
					return fmt.Errorf("folder %d is a project but the API did not return project_id; cannot route delete", id)
				}
				if err := client.Folders().DeleteProject(cmd.Context(), match.ProjectID); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Project %d deleted (project_id=%d)\n", id, match.ProjectID)
				return nil
			}

			if err := client.Folders().Delete(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Folder %d deleted\n", id)
			return nil
		},
	}
}
