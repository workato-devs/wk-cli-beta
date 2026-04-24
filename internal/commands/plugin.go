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
		Args:  requireArgs(1, "plugin name or path is required, e.g.: wk plugins install recipe-lint"),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			var err error

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

			// Validate that plugin.toml exists at the source
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
			if m != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Installed plugin %q (v%s)\n", m.Name, m.Version)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Plugin installed successfully.")
			}
			return nil
		},
	}
}

func newPluginsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
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
				fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed.")
				return nil
			}

			headers := []string{"Name", "Version", "Path"}
			var rows [][]string
			for _, p := range plugins {
				rows = append(rows, []string{p.Name, p.Version, p.Dir})
			}
			return rctx.Formatter.FormatList(cmd.OutOrStdout(), headers, rows)
		},
	}
}

func newPluginsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Args:  requireArgs(1, "plugin name is required, e.g.: wk plugins remove <name>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			registry, err := plugin.NewRegistry()
			if err != nil {
				return err
			}

			if err := registry.Remove(name); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed plugin %q\n", name)
			return nil
		},
	}
}
