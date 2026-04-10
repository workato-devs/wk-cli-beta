package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/plugin"
)

func newRecipesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "recipes",
		Aliases: []string{"recipe"},
		Short:   "Manage Workato recipes",
	}
	cmd.AddCommand(newRecipesListCmd())
	cmd.AddCommand(newRecipesGetCmd())
	cmd.AddCommand(newRecipesStartCmd())
	cmd.AddCommand(newRecipesStopCmd())
	cmd.AddCommand(newRecipesExportCmd())
	cmd.AddCommand(newRecipesImportCmd())
	cmd.AddCommand(newRecipesJobsCmd())
	cmd.AddCommand(newRecipesCopyCmd())
	cmd.AddCommand(newRecipesConnectCmd())
	cmd.AddCommand(newRecipeValidateCmd())
	return cmd
}

func newRecipesListCmd() *cobra.Command {
	var folderID int
	var status string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recipes",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			opts := &api.RecipeListOptions{
				Status: status,
			}
			if cmd.Flags().Changed("folder") {
				opts.FolderID = &folderID
			}

			recipes, err := client.Recipes().List(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, recipes)
			}

			headers := []string{"ID", "NAME", "DESCRIPTION", "FOLDER", "RUNNING", "VERSION"}
			var rows [][]string
			for _, r := range recipes {
				running := "stopped"
				if r.Running {
					running = "running"
				}
				rows = append(rows, []string{
					strconv.Itoa(r.ID),
					r.Name,
					r.Description,
					strconv.Itoa(r.FolderID),
					running,
					strconv.Itoa(r.Version),
				})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().IntVar(&folderID, "folder", 0, "Filter by folder ID")
	cmd.Flags().StringVar(&status, "status", "all", "Filter by status (running, stopped, all)")
	return cmd
}

func newRecipesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get recipe details",
		Args:  cobra.ExactArgs(1),
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
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}

			recipe, err := client.Recipes().Get(cmd.Context(), id)
			if err != nil {
				return err
			}

			if !flagJSON {
				running := "no"
				if recipe.Running {
					running = "yes"
				}
				fmt.Fprintf(os.Stdout, "ID:          %d\n", recipe.ID)
				fmt.Fprintf(os.Stdout, "Name:        %s\n", recipe.Name)
				fmt.Fprintf(os.Stdout, "Description: %s\n", recipe.Description)
				fmt.Fprintf(os.Stdout, "Folder ID:   %d\n", recipe.FolderID)
				fmt.Fprintf(os.Stdout, "Running:     %s\n", running)
				fmt.Fprintf(os.Stdout, "Version:     %d\n", recipe.Version)
				fmt.Fprintf(os.Stdout, "Updated:     %s\n", recipe.UpdatedAt.Format(time.RFC3339))
				return nil
			}
			return rctx.Formatter.Format(os.Stdout, recipe)
		},
	}
}

func newRecipesStartCmd() *cobra.Command {
	var noWait bool
	var pollTimeout int

	cmd := &cobra.Command{
		Use:   "start <id>",
		Short: "Start a recipe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}

			if err := client.Recipes().Start(cmd.Context(), id); err != nil {
				return err
			}

			if noWait {
				fmt.Fprintf(os.Stdout, "Recipe %d start requested\n", id)
				return nil
			}

			// Poll to verify the recipe actually became active.
			pt := pollTimeout
			if pt > 600 {
				pt = 600
			}
			timeout := time.Duration(pt) * time.Second
			const interval = 2 * time.Second
			deadline := time.Now().Add(timeout)

			for time.Now().Before(deadline) {
				recipe, err := client.Recipes().Get(cmd.Context(), id)
				if err != nil {
					return fmt.Errorf("checking recipe status: %w", err)
				}
				if recipe.Running && recipe.Active {
					fmt.Fprintf(os.Stdout, "Recipe %d started and active\n", id)
					return nil
				}
				time.Sleep(interval)
			}

			return fmt.Errorf("recipe %d did not become active within %s", id, timeout)
		},
	}

	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Do not wait for recipe to become active")
	cmd.Flags().IntVar(&pollTimeout, "poll-timeout", 120, "Seconds to wait for recipe to become active (max 600)")
	return cmd
}

func newRecipesStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <id>",
		Short: "Stop a recipe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}

			if err := client.Recipes().Stop(cmd.Context(), id); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Recipe %d stopped\n", id)
			return nil
		},
	}
}

func newRecipesExportCmd() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a recipe as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}

			data, err := client.Recipes().Export(cmd.Context(), id)
			if err != nil {
				return err
			}

			if outputFile != "" {
				if err := os.WriteFile(outputFile, data, 0644); err != nil {
					return fmt.Errorf("writing file: %w", err)
				}
				fmt.Fprintf(os.Stdout, "Recipe %d exported to %s\n", id, outputFile)
				return nil
			}

			_, err = os.Stdout.Write(data)
			return err
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file path")
	return cmd
}

func newRecipesImportCmd() *cobra.Command {
	var folderID int

	cmd := &cobra.Command{
		Use:   "import <path>",
		Short: "Import a recipe from JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}

			recipe, err := client.Recipes().Import(cmd.Context(), folderID, data)
			if err != nil {
				return err
			}

			if !flagJSON {
				running := "no"
				if recipe.Running {
					running = "yes"
				}
				fmt.Fprintf(os.Stdout, "ID:          %d\n", recipe.ID)
				fmt.Fprintf(os.Stdout, "Name:        %s\n", recipe.Name)
				fmt.Fprintf(os.Stdout, "Folder ID:   %d\n", recipe.FolderID)
				fmt.Fprintf(os.Stdout, "Running:     %s\n", running)
				return nil
			}
			return rctx.Formatter.Format(os.Stdout, recipe)
		},
	}

	cmd.Flags().IntVar(&folderID, "folder", 0, "Target folder ID")
	return cmd
}

func newRecipesJobsCmd() *cobra.Command {
	var status string
	var limit int

	cmd := &cobra.Command{
		Use:   "jobs <id>",
		Short: "List recipe jobs",
		Args:  cobra.ExactArgs(1),
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
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}

			opts := &api.JobListOptions{
				Status: status,
				Limit:  limit,
			}

			jobs, err := client.Recipes().ListJobs(cmd.Context(), id, opts)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, jobs)
			}

			headers := []string{"ID", "STATUS", "STARTED AT", "COMPLETED AT"}
			var rows [][]string
			for _, j := range jobs {
				started := ""
				if j.StartedAt != nil {
					started = j.StartedAt.Format(time.RFC3339)
				}
				completed := ""
				if j.CompletedAt != nil {
					completed = j.CompletedAt.Format(time.RFC3339)
				}
				rows = append(rows, []string{
					strconv.Itoa(j.ID),
					j.Status,
					started,
					completed,
				})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&status, "status", "all", "Filter by status (succeeded, failed, all)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of jobs to return")
	return cmd
}

func newRecipesCopyCmd() *cobra.Command {
	var toFolder int

	cmd := &cobra.Command{
		Use:   "copy <id>",
		Short: "Copy a recipe to a folder",
		Args:  cobra.ExactArgs(1),
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
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}

			recipe, err := client.Recipes().Copy(cmd.Context(), id, toFolder)
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, recipe)
			}

			fmt.Fprintf(os.Stdout, "Recipe %d copied to folder %d, new ID: %d\n", id, toFolder, recipe.ID)
			return nil
		},
	}

	cmd.Flags().IntVar(&toFolder, "to-folder", 0, "Target folder ID")
	_ = cmd.MarkFlagRequired("to-folder")
	return cmd
}

func newRecipesConnectCmd() *cobra.Command {
	var adapter string
	var connectionID int

	cmd := &cobra.Command{
		Use:   "update-connection <id>",
		Short: "Update a recipe's connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}

			if err := client.Recipes().Connect(cmd.Context(), id, adapter, connectionID); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Recipe %d connection updated\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&adapter, "adapter", "", "Adapter name")
	cmd.Flags().IntVar(&connectionID, "connection", 0, "Connection ID")
	_ = cmd.MarkFlagRequired("adapter")
	_ = cmd.MarkFlagRequired("connection")
	return cmd
}

// newRecipeValidateCmd creates a "validate" subcommand that delegates to the
// recipe-lint plugin's lint.run method. This is a thin alias so that
// `wk recipes validate <path>` behaves identically to `wk lint <path>`.
func newRecipeValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <path> [paths...]",
		Short: "Validate recipe files (delegates to recipe-lint plugin)",
		Long: `Validate one or more Workato recipe JSON files using the recipe-lint plugin.

This is an alias for "wk lint" — it uses the same tiered validation engine.
The recipe-lint plugin must be installed (wk plugins install <path>).`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := plugin.NewRegistry()
			if err != nil {
				return fmt.Errorf("recipe-lint plugin not installed. Run: wk plugins install <path>")
			}

			pluginDir, err := reg.GetPluginDir("recipe-lint")
			if err != nil {
				return fmt.Errorf("recipe-lint plugin not installed. Run: wk plugins install <path>")
			}

			// Verify the manifest is readable.
			m, err := plugin.LoadManifest(filepath.Join(pluginDir, "plugin.toml"))
			if err != nil {
				return fmt.Errorf("recipe-lint plugin manifest unreadable: %w", err)
			}
			_ = m

			def := &pluginCmdDef{
				Args: []plugin.Arg{{Name: "files", Description: "Recipe files to lint", Required: true}},
				Flags: []plugin.Flag{
					{Name: "tiers", Description: "Lint tier levels to run", Type: "int-array"},
					{Name: "skills-path", Description: "Path to connector skills directory", Type: "string"},
					{Name: "config-path", Description: "Path to lint configuration file", Type: "string"},
				},
			}

			return makePluginRunE(pluginDir, "lint.run", def)(cmd, args)
		},
	}

	cmd.Flags().IntSlice("tiers", nil, "Lint tier levels to run")
	cmd.Flags().String("skills-path", "", "Path to connector skills directory")
	cmd.Flags().String("config-path", "", "Path to lint configuration file")
	return cmd
}
