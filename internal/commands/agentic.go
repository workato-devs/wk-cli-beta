package commands

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/output"
)

func newAgenticCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agentic",
		Short: "Manage Agent Studio resources",
	}
	cmd.AddCommand(newSkillsCmd())
	return cmd
}

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "skills",
		Aliases: []string{"skill"},
		Short:   "Manage agentic skills",
	}
	cmd.AddCommand(newSkillsListCmd())
	cmd.AddCommand(newSkillsGetCmd())
	cmd.AddCommand(newSkillsCreateCmd())
	return cmd
}

func newSkillsListCmd() *cobra.Command {
	var page, perPage int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agentic skills",
		Example: `  wk agentic skills list
  wk agentic skills list --json`,
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
			skills, err := client.Skills().List(cmd.Context(), opts)
			if err != nil {
				return err
			}

			headers := []string{"ID", "NAME", "RECIPE ID", "FOLDER ID", "PROJECT ID", "RUNNING"}
			var rows [][]string
			for _, s := range skills {
				running := "no"
				if s.Running {
					running = "yes"
				}
				rows = append(rows, []string{
					s.ID,
					s.Name,
					strconv.Itoa(s.RecipeID),
					strconv.Itoa(s.FolderID),
					strconv.Itoa(s.ProjectID),
					running,
				})
			}
			meta := output.PageMeta{Page: page, PerPage: perPage, HasNext: perPage > 0 && len(skills) == perPage}
			return rctx.Formatter.FormatPage(os.Stdout, skills, headers, rows, meta)
		},
	}

	cmd.Flags().IntVar(&page, "page", 0, "Page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newSkillsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get an agentic skill by ID",
		Example: `  wk agentic skills get skl-abc123 --json`,
		Args:  requireArgs(1, "skill ID is required, e.g.: wk agentic skills get <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			skill, err := client.Skills().Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, skill)
			}

			running := "no"
			if skill.Running {
				running = "yes"
			}
			headers := []string{"ID", "NAME", "RECIPE ID", "FOLDER ID", "PROJECT ID", "RUNNING"}
			rows := [][]string{{
				skill.ID,
				skill.Name,
				strconv.Itoa(skill.RecipeID),
				strconv.Itoa(skill.FolderID),
				strconv.Itoa(skill.ProjectID),
				running,
			}}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}
}

func newSkillsCreateCmd() *cobra.Command {
	var recipeID int

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an agentic skill from a recipe",
		Example: `  wk agentic skills create --recipe-id 12345 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			skill, err := client.Skills().Create(cmd.Context(), recipeID)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, skill)
			}

			fmt.Fprintf(os.Stderr, "Created skill %q (ID: %s)\n", skill.Name, skill.ID)
			return nil
		},
	}

	cmd.Flags().IntVar(&recipeID, "recipe-id", 0, "Recipe ID to create skill from")
	_ = cmd.MarkFlagRequired("recipe-id")
	return cmd
}
