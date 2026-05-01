package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/plugin"
)

func newPluginsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plugins",
		Aliases: []string{"plugin"},
		Short:   "Manage wk plugins",
	}

	cmd.AddCommand(newPluginsInstallCmd())
	cmd.AddCommand(newPluginsListCmd())
	cmd.AddCommand(newPluginsRemoveCmd())

	return cmd
}

func newPluginsInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <name-or-path>",
		Short: "Install a plugin by name (from $PATH) or local path",
		Example: `  wk plugins install recipe-lint
  wk plugins install ./my-plugin --json`,
		Args:  requireArgs(1, "plugin name or path is required, e.g.: wk plugins install recipe-lint"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			source := args[0]

			if !filepath.IsAbs(source) && !strings.Contains(source, string(filepath.Separator)) && source != "." && source != ".." {
				binPath, lookErr := exec.LookPath(source)
				if lookErr != nil {
					return fmt.Errorf("plugin %q not found on $PATH; to install from a local directory, use: wk plugins install ./%s", source, source)
				}
				binPath, err = filepath.EvalSymlinks(binPath)
				if err != nil {
					return fmt.Errorf("resolving symlink: %w", err)
				}
				source, err = filepath.Abs(filepath.Dir(binPath))
			} else {
				source, err = filepath.Abs(source)
			}
			if err != nil {
				return fmt.Errorf("resolving path: %w", err)
			}

			manifestPath := filepath.Join(source, "plugin.toml")
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				return fmt.Errorf("no plugin.toml found at %s", source)
			}

			registry, err := plugin.NewRegistry()
			if err != nil {
				return err
			}

			if err := registry.Install(source); err != nil {
				return err
			}

			m, _ := plugin.LoadManifest(manifestPath)
			if flagJSON {
				result := map[string]string{"status": "installed", "path": source}
				if m != nil {
					result["name"] = m.Name
					result["version"] = m.Version
				}
				return rctx.Formatter.Format(os.Stdout, result)
			}

			if m != nil {
				fmt.Fprintf(os.Stderr, "Installed plugin %q (v%s)\n", m.Name, m.Version)
			} else {
				fmt.Fprintln(os.Stderr, "Plugin installed successfully.")
			}
			return nil
		},
	}
}

func newPluginsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Example: `  wk plugins list
  wk plugins list --json`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			registry, err := plugin.NewRegistry()
			if err != nil {
				return err
			}

			plugins, err := registry.List()
			if err != nil {
				return err
			}

			if len(plugins) == 0 {
				fmt.Fprintln(os.Stderr, "No plugins installed.")
				return nil
			}

			headers := []string{"Name", "Version", "Path"}
			var rows [][]string
			for _, p := range plugins {
				rows = append(rows, []string{p.Name, p.Version, p.Dir})
			}
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}
}

func newPluginsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Example: `  wk plugins remove recipe-lint`,
		Args:  requireArgs(1, "plugin name is required, e.g.: wk plugins remove <name>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			registry, err := plugin.NewRegistry()
			if err != nil {
				return err
			}

			if err := registry.Remove(name); err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, map[string]string{"status": "removed", "name": name})
			}
			fmt.Fprintf(os.Stderr, "Removed plugin %q\n", name)
			return nil
		},
	}
}
