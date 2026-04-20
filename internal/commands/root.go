package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
	"github.com/workato-devs/wk-cli-beta/internal/output"
	"github.com/workato-devs/wk-cli-beta/internal/plugin"
)

// Version info set by main via SetVersionInfo.
var (
	versionStr = "dev"
	commitStr  = "none"
	dateStr    = "unknown"
)

// SetVersionInfo is called from main.go to inject ldflags values.
func SetVersionInfo(version, commit, date string) {
	versionStr = version
	commitStr = commit
	dateStr = date
}

// RunContext carries resolved dependencies into every command handler.
// No global state — everything a command needs is here.
type RunContext struct {
	Config         *config.Config
	ProjectRoot    string
	AuthStore      auth.CredentialStore
	APIClient      api.Client
	Formatter      output.Formatter
	Profile        *auth.Profile
	PluginRegistry *plugin.Registry
	Verbose        bool
	Quiet          bool
}

var (
	flagJSON      bool
	flagVerbose   bool
	flagQuiet     bool
	flagProfile   string
	flagStoreType string
	flagNoColor   bool
	flagTimeout   int
	flagNoInput   bool
)

// NewRootCmd builds the root cobra command with all global flags.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "wk",
		Short: "Workato CLI - manage your Workato workspace from the terminal",
		Long: `wk is the official Workato CLI tool for managing recipes, connections,
sync operations, and plugins from your terminal or CI/CD pipeline.

Every command supports --json for machine-readable output.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pf := root.PersistentFlags()
	// -v is intentionally unassigned: --verify (init) and --verbose both
	// have a legitimate claim to it and the ambiguity isn't worth the
	// keystroke. --verbose stays long-only.
	pf.BoolVarP(&flagJSON, "json", "j", false, "Output as JSON")
	pf.BoolVar(&flagVerbose, "verbose", false, "Enable verbose/debug logging")
	pf.BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress non-essential output")
	pf.StringVarP(&flagProfile, "profile", "p", "", "Override active auth profile")
	pf.StringVar(&flagStoreType, "store-type", "", "Override credential store backend (keychain|file)")
	pf.BoolVar(&flagNoColor, "no-color", false, "Disable color output")
	pf.IntVar(&flagTimeout, "timeout", config.DefaultTimeout, "API timeout in seconds")
	// --no-input forces non-interactive mode for commands that would
	// otherwise prompt (currently only `wk init`). Accepted everywhere for
	// a consistent scripting contract; a no-op where nothing prompts.
	pf.BoolVar(&flagNoInput, "no-input", false, "Force non-interactive mode (fail on missing required flags instead of prompting)")

	return root
}

// BuildRunContext resolves dependencies for a command invocation.
func BuildRunContext(cmd *cobra.Command) (*RunContext, error) {
	format := "text"
	if flagJSON {
		format = "json"
	}

	rctx := &RunContext{
		Formatter: output.NewFormatter(format),
		Verbose:   flagVerbose,
		Quiet:     flagQuiet,
	}

	// Try to load project config (optional — not all commands need it)
	cwd, err := os.Getwd()
	if err == nil {
		if projectRoot, err := config.FindProjectRoot(cwd); err == nil {
			cfg, err := config.Load(config.ProjectConfigPath(projectRoot))
			if err == nil {
				rctx.Config = cfg
				rctx.ProjectRoot = projectRoot
			}
		}
	}

	// Initialize plugin registry (best-effort — not all environments have $HOME)
	if reg, err := plugin.NewRegistry(); err == nil {
		rctx.PluginRegistry = reg
	}

	return rctx, nil
}

// requireArgs returns a cobra.PositionalArgs validator that checks for exactly n
// args and returns a user-friendly message instead of the generic cobra error.
func requireArgs(n int, msg string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("%s\n\nUsage: %s", msg, cmd.UseLine())
		}
		return cobra.ExactArgs(n)(cmd, args)
	}
}

// Execute runs the root command.
func Execute(ctx context.Context) int {
	root := NewRootCmd()
	registerAllCommands(root)

	if err := root.ExecuteContext(ctx); err != nil {
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		return 1
	}
	return 0
}

// registerAllCommands wires all command groups into the root command.
// This is the single integration point — each command file provides a
// New*Cmd() function that is registered here.
func registerAllCommands(root *cobra.Command) {
	root.AddCommand(newVersionCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newLinkCmd())
	root.AddCommand(newAuthCmd())
	root.AddCommand(newRecipesCmd())
	root.AddCommand(newConnectionsCmd())
	root.AddCommand(newPullCmd())
	root.AddCommand(newPushCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newDiffCmd())
	root.AddCommand(newCloneCmd())
	root.AddCommand(newPluginsCmd())
	root.AddCommand(newFoldersCmd())
	root.AddCommand(newTagsCmd())
	root.AddCommand(newAPICmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newWorkspaceCmd())
	root.AddCommand(newConnectorsCmd())
	root.AddCommand(newSyncCmd())
	registerPluginCommands(root)
}

// hasCommand checks whether root already has a subcommand with the given name.
func hasCommand(root *cobra.Command, name string) bool {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

// pluginCmdDef holds the arg/flag declarations for a plugin command so that
// makePluginRunE can build a structured params object instead of sending a raw
// string array.
type pluginCmdDef struct {
	Args  []plugin.Arg
	Flags []plugin.Flag
}

// makePluginRunE creates a RunE function that loads a plugin and calls a method.
// If def is non-nil and declares args or flags, the positional args and flag
// values are assembled into a JSON object; otherwise args are passed as a raw
// string array for backwards compatibility.
func makePluginRunE(pluginDir, method string, def *pluginCmdDef) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		host := plugin.NewPluginHost()
		defer host.StopAll()

		if err := host.Load(pluginDir); err != nil {
			return fmt.Errorf("loading plugin: %w", err)
		}

		m, _ := plugin.LoadManifest(filepath.Join(pluginDir, "plugin.toml"))
		if m == nil {
			return fmt.Errorf("cannot read plugin manifest")
		}

		params := buildPluginParams(cmd, args, def)

		result, err := host.Execute(m.Name, method, params)
		if err != nil {
			return err
		}

		if flagJSON {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}
			return rctx.Formatter.Format(os.Stdout, json.RawMessage(result))
		}

		var m2 map[string]any
		if json.Unmarshal(result, &m2) == nil {
			for k, v := range m2 {
				fmt.Fprintf(os.Stdout, "%s: %v\n", k, v)
			}
			return nil
		}

		fmt.Fprintf(os.Stdout, "%s\n", string(result))
		return nil
	}
}

// buildPluginParams constructs the RPC params value from positional args and
// cobra flags. When no arg/flag declarations exist it returns the raw []string
// for backwards compatibility.
func buildPluginParams(cmd *cobra.Command, args []string, def *pluginCmdDef) any {
	if def == nil || (len(def.Args) == 0 && len(def.Flags) == 0) {
		return args
	}

	obj := make(map[string]any)

	// Map positional args using the first declared arg name.
	if len(def.Args) > 0 {
		obj[def.Args[0].Name] = args
	}

	// Map each declared flag that was explicitly set on the command line.
	for _, f := range def.Flags {
		if !cmd.Flags().Changed(f.Name) {
			continue
		}
		key := flagToJSON(f.Name)
		switch f.Type {
		case "int":
			val, _ := cmd.Flags().GetInt(f.Name)
			obj[key] = val
		case "int-array":
			val, _ := cmd.Flags().GetIntSlice(f.Name)
			obj[key] = val
		case "bool":
			val, _ := cmd.Flags().GetBool(f.Name)
			obj[key] = val
		case "string-array":
			val, _ := cmd.Flags().GetStringSlice(f.Name)
			obj[key] = val
		default: // "string" or empty
			val, _ := cmd.Flags().GetString(f.Name)
			obj[key] = val
		}
	}

	return obj
}

// flagToJSON converts kebab-case flag names to snake_case JSON field names.
func flagToJSON(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

// registerPluginCommands discovers installed plugins and registers their
// commands on the root command.
func registerPluginCommands(root *cobra.Command) {
	registry, err := plugin.NewRegistry()
	if err != nil {
		return
	}

	plugins, err := registry.List()
	if err != nil || len(plugins) == 0 {
		return
	}

	for _, p := range plugins {
		m, err := plugin.LoadManifest(filepath.Join(p.Dir, "plugin.toml"))
		if err != nil {
			continue
		}

		for _, pcmd := range m.Commands {
			if hasCommand(root, pcmd.Name) {
				continue
			}

			if pcmd.Method != "" {
				def := &pluginCmdDef{Args: pcmd.Args, Flags: pcmd.Flags}
				cmd := &cobra.Command{
					Use:   pcmd.Name,
					Short: pcmd.Description,
					RunE:  makePluginRunE(p.Dir, pcmd.Method, def),
				}
				registerPluginFlags(cmd, pcmd.Flags)
				root.AddCommand(cmd)
			} else if len(pcmd.Subcommands) > 0 {
				parent := &cobra.Command{
					Use:   pcmd.Name,
					Short: pcmd.Description,
				}
				for _, sub := range pcmd.Subcommands {
					def := &pluginCmdDef{Args: sub.Args, Flags: sub.Flags}
					child := &cobra.Command{
						Use:   sub.Name,
						Short: sub.Description,
						RunE:  makePluginRunE(p.Dir, sub.Method, def),
					}
					registerPluginFlags(child, sub.Flags)
					parent.AddCommand(child)
				}
				root.AddCommand(parent)
			}
		}
	}
}

// registerPluginFlags registers declared flags on a cobra command.
func registerPluginFlags(cmd *cobra.Command, flags []plugin.Flag) {
	for _, f := range flags {
		switch f.Type {
		case "int":
			cmd.Flags().Int(f.Name, 0, f.Description)
		case "int-array":
			cmd.Flags().IntSlice(f.Name, nil, f.Description)
		case "bool":
			cmd.Flags().Bool(f.Name, false, f.Description)
		case "string-array":
			cmd.Flags().StringSlice(f.Name, nil, f.Description)
		default: // "string" or empty
			cmd.Flags().String(f.Name, f.Default, f.Description)
		}
	}
}
