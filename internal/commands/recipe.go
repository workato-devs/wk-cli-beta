package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/config"
	"github.com/workato-devs/wk-cli-beta/internal/output"
	"github.com/workato-devs/wk-cli-beta/internal/plugin"
	"github.com/workato-devs/wk-cli-beta/internal/sync"
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
	cmd.AddCommand(newRecipesUpdateCmd())
	cmd.AddCommand(newRecipesDeleteCmd())
	cmd.AddCommand(newRecipesJobsCmd())
	cmd.AddCommand(newRecipesCopyCmd())
	cmd.AddCommand(newRecipesConnectCmd())
	cmd.AddCommand(newRecipeValidateCmd())
	cmd.AddCommand(newRecipesVersionsCmd())
	return cmd
}

func newRecipesListCmd() *cobra.Command {
	var folderID int
	var status string
	var page, perPage int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recipes",
		Example: `  wk recipes list
  wk recipes list --folder 123 --status running
  wk recipes list --json --page 1 --per-page 50`,
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
				Status:  status,
				Page:    page,
				PerPage: perPage,
			}
			if cmd.Flags().Changed("folder") {
				opts.FolderID = &folderID
			}

			recipes, err := client.Recipes().List(cmd.Context(), opts)
			if err != nil {
				return err
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
			meta := output.PageMeta{Page: page, PerPage: perPage, HasNext: perPage > 0 && len(recipes) == perPage}
			return rctx.Formatter.FormatPage(os.Stdout, recipes, headers, rows, meta)
		},
	}

	cmd.Flags().IntVar(&folderID, "folder", 0, "Filter by folder ID")
	cmd.Flags().StringVar(&status, "status", "all", "Filter by status (running, stopped, all)")
	cmd.Flags().IntVar(&page, "page", 0, "Page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newRecipesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get recipe details",
		Example: `  wk recipes get 12345
  wk recipes get 12345 --json`,
		Args:  requireArgs(1, "recipe ID is required, e.g.: wk recipes get <id>"),
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
		Example: `  wk recipes start 12345
  wk recipes start 12345 --no-wait`,
		Args:  requireArgs(1, "recipe ID is required, e.g.: wk recipes start <id>"),
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
				fmt.Fprintf(os.Stderr, "Recipe %d start requested\n", id)
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
				if recipe.Running {
					fmt.Fprintf(os.Stderr, "Recipe %d started and running\n", id)
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
		Use:     "stop <id>",
		Short:   "Stop a recipe",
		Example: `  wk recipes stop 12345`,
		Args:  requireArgs(1, "recipe ID is required, e.g.: wk recipes stop <id>"),
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

			fmt.Fprintf(os.Stderr, "Recipe %d stopped\n", id)
			return nil
		},
	}
}

func newRecipesExportCmd() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a recipe as JSON",
		Example: `  wk recipes export 12345
  wk recipes export 12345 -o recipe.json`,
		Args:  requireArgs(1, "recipe ID is required, e.g.: wk recipes export <id>"),
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
				fmt.Fprintf(os.Stderr, "Recipe %d exported to %s\n", id, outputFile)
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
		Example: `  wk recipes import recipe.json --folder 123
  wk recipes import recipe.json --json`,
		Args:  requireArgs(1, "file path is required, e.g.: wk recipes import <path>"),
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
		Use:   "jobs <recipe-id>",
		Short: "List recipe jobs",
		Example: `  wk recipes jobs 12345
  wk recipes jobs 12345 --status failed --limit 10 --json`,
		Args: cobra.MinimumNArgs(1),
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

			headers := []string{"ID", "STATUS", "STARTED AT", "COMPLETED AT", "ERROR"}
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
				errMsg := ""
				if j.Error != nil {
					errMsg = *j.Error
				}
				rows = append(rows, []string{
					j.ID,
					j.Status,
					started,
					completed,
					errMsg,
				})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}

	cmd.Flags().StringVar(&status, "status", "all", "Filter by status (succeeded, failed, all)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of jobs to return")
	cmd.AddCommand(newRecipesJobsGetCmd())
	return cmd
}

func newRecipesJobsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <recipe-id> <job-id>",
		Short: "Get details for a single job, including step traces",
		Example: `  wk recipes jobs get 12345 67890
  wk recipes jobs get 12345 67890 --json`,
		Args: requireArgs(2, "recipe ID and job ID are required, e.g.: wk recipes jobs get <recipe-id> <job-id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}

			recipeID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}

			detail, err := client.Recipes().GetJob(cmd.Context(), recipeID, args[1])
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, detail)
			}

			fmt.Fprintf(os.Stdout, "Job ID:     %s\n", detail.ID)
			fmt.Fprintf(os.Stdout, "Recipe ID:  %d\n", detail.RecipeID)
			fmt.Fprintf(os.Stdout, "Status:     %s\n", detail.Status)
			if detail.StartedAt != nil {
				fmt.Fprintf(os.Stdout, "Started:    %s\n", detail.StartedAt.Format(time.RFC3339))
			}
			if detail.CompletedAt != nil {
				fmt.Fprintf(os.Stdout, "Completed:  %s\n", detail.CompletedAt.Format(time.RFC3339))
			}
			if detail.Error != nil {
				fmt.Fprintf(os.Stdout, "Error:      %s\n", *detail.Error)
			}

			if len(detail.Lines) > 0 {
				fmt.Fprintln(os.Stdout)
				headers := []string{"STEP", "ADAPTER", "OPERATION", "STARTED AT", "COMPLETED AT"}
				var rows [][]string
				for _, line := range detail.Lines {
					started, completed := "", ""
					if line.LineStat != nil {
						if line.LineStat.StartedAt != nil {
							started = line.LineStat.StartedAt.Format(time.RFC3339)
						}
						if line.LineStat.CompletedAt != nil {
							completed = line.LineStat.CompletedAt.Format(time.RFC3339)
						}
					}
					rows = append(rows, []string{
						strconv.Itoa(line.RecipeLineNumber),
						line.AdapterName,
						line.AdapterOperation,
						started,
						completed,
					})
				}
				return rctx.Formatter.FormatList(os.Stdout, headers, rows)
			}
			return nil
		},
	}
}

func newRecipesCopyCmd() *cobra.Command {
	var toFolder int

	cmd := &cobra.Command{
		Use:   "copy <id>",
		Short: "Copy a recipe to a folder",
		Example: `  wk recipes copy 12345 --to-folder 678
  wk recipes copy 12345 --to-folder 678 --json`,
		Args:  requireArgs(1, "recipe ID is required, e.g.: wk recipes copy <id>"),
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

			fmt.Fprintf(os.Stderr, "Recipe %d copied to folder %d, new ID: %d\n", id, toFolder, recipe.ID)
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
		Example: `  wk recipes update-connection 12345 --adapter salesforce --connection 678`,
		Args:  requireArgs(1, "recipe ID is required, e.g.: wk recipes update-connection <id>"),
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

			fmt.Fprintf(os.Stderr, "Recipe %d connection updated\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&adapter, "adapter", "", "Adapter name")
	cmd.Flags().IntVar(&connectionID, "connection", 0, "Connection ID")
	_ = cmd.MarkFlagRequired("adapter")
	_ = cmd.MarkFlagRequired("connection")
	return cmd
}

// newRecipesUpdateCmd updates an existing recipe's code/config from a
// local JSON file. Uses PUT /recipes/{id} — the create/update split
// mirrors the Workato API, which treats POST as create and PUT as update.
// Pre-flights with Get() to block updates on running recipes, since the
// API rejects them and the resulting error is opaque.
func newRecipesUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id> <path>",
		Short: "Update an existing recipe from a JSON file",
		Long: `Replace an existing recipe's code/config with the contents of a local JSON
file via PUT /api/recipes/:id. The recipe must be stopped — the API rejects
updates to running recipes. Use "wk recipes stop <id>" first if needed.`,
		Example: `  wk recipes update 12345 recipe.json`,
		Args: requireArgs(2, "recipe ID and file path are required, e.g.: wk recipes update <id> <path>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}
			data, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}

			existing, gerr := client.Recipes().Get(cmd.Context(), id)
			if gerr != nil {
				return fmt.Errorf("fetching recipe %d: %w", id, gerr)
			}
			if existing.Running {
				return fmt.Errorf("recipe %d is running; stop it first (wk recipes stop %d)", id, id)
			}

			if err := client.Recipes().Update(cmd.Context(), id, data); err != nil {
				return err
			}

			updated, gerr := client.Recipes().Get(cmd.Context(), id)
			if gerr != nil {
				// Update succeeded; we just can't show the new version number.
				fmt.Fprintf(os.Stderr, "Recipe %d updated\n", id)
				return nil
			}
			fmt.Fprintf(os.Stderr, "Recipe %d updated (version %d)\n", id, updated.Version)
			return nil
		},
	}
	return cmd
}

// newRecipesDeleteCmd removes a recipe from the Workato workspace (and,
// when running inside a wk project, cleans up the corresponding local
// .recipe.json + .meta.json sidecar pair by matching the recipe's name).
//
// The pull-side zip doesn't carry server-side IDs — only the recipe's
// name — so local-cleanup happens by name. When the user passes an ID,
// we fetch the recipe's name via a single GET before deleting.
//
// --local-only skips the DELETE. It still uses GET to resolve ID→name
// unless the caller also passed --name (in which case no API is hit).
// Useful for cleaning up orphans left by earlier wk pull runs that
// pre-date the A3 deletion-reconciliation behavior.
//
// --name matches local files directly and resolves the server ID via
// List for the DELETE. Ambiguous matches are an error — fall back to ID.
func newRecipesDeleteCmd() *cobra.Command {
	var (
		flagLocalOnly bool
		flagByName    string
	)
	cmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a recipe (server and/or local)",
		Long: `Delete a recipe from the Workato workspace. When invoked inside a wk
project, also removes the matching local .recipe.json and .meta.json
files — matched by the recipe's name, since pull zips don't carry
server-side IDs.

Use --name to delete by recipe name instead of ID (exact match).
Use --local-only to skip the server delete and only clean up local files.

Pure-offline variant: "wk recipes delete --name <name> --local-only"
does no API call at all and is safe when the recipe is already gone.`,
		Example: `  wk recipes delete 12345
  wk recipes delete --name "New lead sync" --local-only`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagByName == "" && len(args) == 0 {
				return fmt.Errorf("recipe ID or --name is required")
			}
			if flagByName != "" && len(args) > 0 {
				return fmt.Errorf("pass either an ID or --name, not both")
			}

			var (
				id         int
				recipeName = flagByName
			)

			// ID supplied — resolve to a recipe name via GET so local
			// cleanup has something to match against. This also lets the
			// server-delete step emit a useful "deleting recipe N (name)"
			// message later.
			if len(args) == 1 {
				parsed, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid recipe ID: %s", args[0])
				}
				id = parsed

				client, _, err := resolveAPIClient(cmd)
				if err != nil {
					return err
				}
				recipe, gerr := client.Recipes().Get(cmd.Context(), id)
				if gerr != nil {
					// Most likely the recipe is already gone server-side.
					// For --local-only the user can retry with --name.
					if flagLocalOnly {
						return fmt.Errorf("fetching recipe %d to resolve name: %w\n(pass --name <name> to clean up without an API lookup)", id, gerr)
					}
					return fmt.Errorf("fetching recipe %d: %w", id, gerr)
				}
				recipeName = recipe.Name
			}

			// Name supplied — resolve the ID via List (only needed when
			// we'll actually hit the DELETE endpoint).
			if flagByName != "" && !flagLocalOnly {
				client, _, err := resolveAPIClient(cmd)
				if err != nil {
					return err
				}
				resolved, rerr := resolveRecipeIDByName(cmd, client, flagByName)
				if rerr != nil {
					return rerr
				}
				id = resolved
			}

			if !flagLocalOnly {
				client, _, err := resolveAPIClient(cmd)
				if err != nil {
					return err
				}
				if err := client.Recipes().Delete(cmd.Context(), id); err != nil {
					return fmt.Errorf("deleting recipe %d: %w", id, err)
				}
			}

			removed := cleanupLocalRecipeFiles(recipeName)
			if flagLocalOnly {
				if len(removed) == 0 {
					return fmt.Errorf("no local files found for recipe name %q", recipeName)
				}
				fmt.Fprintf(os.Stderr, "Recipe %q: removed %d local file(s)\n", recipeName, len(removed))
				for _, p := range removed {
					fmt.Fprintf(os.Stderr, "  %s\n", p)
				}
				return nil
			}

			if len(removed) > 0 {
				fmt.Fprintf(os.Stderr, "Recipe %d (%q) deleted (also removed %d local file(s))\n", id, recipeName, len(removed))
			} else {
				fmt.Fprintf(os.Stderr, "Recipe %d (%q) deleted\n", id, recipeName)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagLocalOnly, "local-only", false, "Skip the server delete; only remove local files")
	cmd.Flags().StringVar(&flagByName, "name", "", "Operate on the recipe with this exact name")
	return cmd
}

// resolveRecipeIDByName looks up a recipe by exact name via the list API.
// Ambiguous matches error out — the user should pick an ID explicitly.
func resolveRecipeIDByName(cmd *cobra.Command, client api.Client, name string) (int, error) {
	recipes, err := client.Recipes().List(cmd.Context(), &api.RecipeListOptions{})
	if err != nil {
		return 0, fmt.Errorf("listing recipes for --name lookup: %w", err)
	}
	var matches []api.Recipe
	for _, r := range recipes {
		if r.Name == name {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("no recipe found with name %q", name)
	case 1:
		return matches[0].ID, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, r := range matches {
			ids = append(ids, strconv.Itoa(r.ID))
		}
		return 0, fmt.Errorf("recipe name %q is ambiguous — %d matches (IDs: %s); pass the ID explicitly", name, len(matches), strings.Join(ids, ", "))
	}
}

// cleanupLocalRecipeFiles best-effort removes the .recipe.json and its
// .meta.json sidecar for the given recipe name. Matches by the
// recipe_name field inside each meta — names are the stable, portable
// identifier across environments, whereas server-side IDs are
// per-environment and ephemeral (an ID that identifies a recipe in dev
// is a different recipe in prod). The pull zip format reflects this:
// it carries names, not IDs.
//
// Runs only inside a wk project — elsewhere returns nil immediately.
// Returns the list of files removed so the caller can report them.
func cleanupLocalRecipeFiles(recipeName string) []string {
	if recipeName == "" {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	root, err := config.FindProjectRoot(cwd)
	if err != nil {
		return nil
	}
	cfg, err := config.Load(config.ProjectConfigPath(root))
	if err != nil {
		return nil
	}

	var removed []string
	for _, entry := range cfg.Sync {
		localDir := filepath.Join(root, entry.LocalPath)
		metas, err := sync.FindMetaFiles(root, localDir)
		if err != nil {
			continue
		}
		for rel, meta := range metas {
			if !metaMatchesRecipe(meta, recipeName) {
				continue
			}
			assetAbs := filepath.Join(localDir, rel)
			metaAbs, mperr := sync.MetaPath(root, assetAbs)
			if mperr != nil {
				continue
			}
			if err := os.Remove(assetAbs); err == nil {
				removed = append(removed, assetAbs)
			}
			if err := os.Remove(metaAbs); err == nil {
				removed = append(removed, metaAbs)
			}
		}
	}
	return removed
}

// metaMatchesRecipe returns true when the AssetMeta identifies the asset
// as the recipe with the given name. Matching is exact on meta.RecipeName
// — the Workato package-manifest zip carries recipe names but not IDs
// (and IDs are per-environment anyway), so name is the only key that
// round-trips cleanly across dev/prod.
//
// Legacy metas written before RecipeName existed return false here; the
// user remedy is a single wk pull, which re-writes metas with the name.
func metaMatchesRecipe(meta *sync.AssetMeta, recipeName string) bool {
	if meta == nil || meta.Type != "recipe" {
		return false
	}
	return meta.RecipeName != "" && meta.RecipeName == recipeName
}

// newRecipesVersionsCmd is the parent of the versions-management subtree.
// Calling it without a subcommand (or with "list") prints the version
// history for a given recipe.
func newRecipesVersionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions <recipe_id>",
		Short: "Manage recipe version history",
		Example: `  wk recipes versions 12345
  wk recipes versions 12345 --json`,
		Args:  cobra.MinimumNArgs(1),
		RunE:  runRecipesVersionsList,
	}
	cmd.AddCommand(newRecipesVersionsCommentCmd())
	return cmd
}

func runRecipesVersionsList(cmd *cobra.Command, args []string) error {
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

	versions, err := client.Recipes().ListVersions(cmd.Context(), id, 0, 0)
	if err != nil {
		return err
	}

	if flagJSON {
		return rctx.Formatter.Format(os.Stdout, versions)
	}

	headers := []string{"VERSION", "ID", "AUTHOR", "COMMENT", "CREATED AT"}
	rows := make([][]string, 0, len(versions))
	for _, v := range versions {
		comment := "—"
		if v.Comment != nil && *v.Comment != "" {
			comment = *v.Comment
		}
		rows = append(rows, []string{
			strconv.Itoa(v.VersionNo),
			strconv.Itoa(v.ID),
			v.AuthorName,
			comment,
			v.CreatedAt.Format(time.RFC3339),
		})
	}
	return rctx.Formatter.FormatList(os.Stdout, headers, rows)
}

func newRecipesVersionsCommentCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "comment <recipe_id> <version_id> <comment>",
		Short:   "Set or update the comment on a recipe version",
		Example: `  wk recipes versions comment 12345 42 "Fixed connection timeout"`,
		Args:  requireArgs(3, "recipe ID, version ID, and comment are required, e.g.: wk recipes versions comment <recipe_id> <version_id> <comment>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := resolveAPIClient(cmd)
			if err != nil {
				return err
			}
			recipeID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid recipe ID: %s", args[0])
			}
			versionID, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid version ID: %s", args[1])
			}
			comment := args[2]

			// The PATCH response shape varies (sometimes the wrapper omits
			// the ID), so report from the arg we already have. The returned
			// *RecipeVersion is still available for future JSON output.
			if _, err := client.Recipes().UpdateVersionComment(cmd.Context(), recipeID, versionID, comment); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Version %d comment updated\n", versionID)
			return nil
		},
	}
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
		Example: `  wk recipes validate ./recipes/
  wk recipes validate my-recipe.recipe.json --tiers 0,1 --json`,
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
