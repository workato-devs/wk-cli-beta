package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
	wkerrors "github.com/workato-devs/wk-cli-beta/internal/errors"
)

// resolveVerifyClient builds an API client for the named profile, used by
// init --verify. This honors StoreType routing (ADR-006 Sub-decision 6) via
// the shared resolveProfileAndCred helper.
func resolveVerifyClient(cmd *cobra.Command, profileName string) (api.Client, error) {
	profile, cred, err := resolveProfileAndCred(cmd.Context(), profileName)
	if err != nil {
		return nil, err
	}

	var opts []api.ClientOption
	if flagVerbose {
		opts = append(opts, api.WithVerbose(true))
	}

	client := api.NewHTTPClient(profile.BaseURL+config.APIPathPrefix, cred.Token, opts...)
	return client, nil
}

// verifyServerPath walks the Workato folder hierarchy to confirm that
// serverPath exists. Returns nil on success or a descriptive error.
func verifyServerPath(cmd *cobra.Command, client api.Client, serverPath string) error {
	parts := strings.Split(strings.Trim(serverPath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("empty server path")
	}

	// Strip implicit root folder "All projects" if present.
	if strings.EqualFold(parts[0], "All projects") {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return nil // root folder always exists
	}

	folders := client.Folders()

	// Fetch the full folder list once (unfiltered) so we can resolve
	// top-level workspace folders whose parent_id is the implicit home
	// folder — the API never returns the home folder itself, so filtering
	// by parent_id=nil would find nothing.
	allFolders, err := folders.List(cmd.Context(), nil)
	if err != nil {
		return fmt.Errorf("verifying server path: %w", err)
	}

	var parentID *int
	for _, name := range parts {
		found := false
		for _, f := range allFolders {
			if !strings.EqualFold(f.Name, name) {
				continue
			}
			// For the first segment, match by name only (top-level folders
			// sit under the implicit home folder whose ID we don't know).
			// For deeper segments, also require the parent ID to match.
			if parentID != nil && (f.ParentID == nil || *f.ParentID != *parentID) {
				continue
			}
			id := f.ID
			parentID = &id
			found = true
			break
		}
		if !found {
			return fmt.Errorf("server path %q not found: folder %q does not exist", serverPath, name)
		}
	}
	return nil
}

func newInitCmd() *cobra.Command {
	var (
		flagName        string
		flagInitProfile string
		flagServerPath  string
		flagLocalPath   string
		flagVerify      bool
		flagInitNoInput bool
		flagOverwrite   bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new wk project",
		Long: `Create a new wk project container in the current directory.

The layout is:

    <cwd>/<name>/
    ├── .wk/
    │   ├── wk.toml        (project config)
    │   └── .gitignore     (self-ignores .wk/ contents; CLI never touches the
    │                       project-root .gitignore)
    └── ...                (asset directories populated by pull/clone)

Non-interactive mode (detected via --json, --no-input, or a non-TTY stdin)
requires --name and --profile explicitly, and requires --overwrite when
replacing an existing project. Mirrors the contract used by 'wk auth login'.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}

			// Nesting guard: prevent creating a project inside an existing one.
			if projectRoot, err := config.FindProjectRoot(cwd); err == nil {
				return fmt.Errorf("%w at %s — run from outside the project directory", wkerrors.ErrNestedProject, projectRoot)
			}

			name := flagName
			profile := flagInitProfile
			interactive := isInteractiveStdin() && !flagInitNoInput && !flagJSON

			// In non-interactive mode, validate required flags upfront so
			// no prompt label ever reaches the terminal (mirrors auth login).
			if !interactive {
				var missing []string
				if name == "" {
					missing = append(missing, "--name")
				}
				if profile == "" {
					missing = append(missing, "--profile")
				}
				if len(missing) > 0 {
					return fmt.Errorf("%s required in non-interactive mode (detected via --json, --no-input, or non-TTY stdin)",
						strings.Join(missing, " and "))
				}
			} else {
				reader := bufio.NewReader(os.Stdin)
				if name == "" {
					fmt.Print("Project name: ")
					name, _ = reader.ReadString('\n')
					name = strings.TrimSpace(name)
					if name == "" {
						return fmt.Errorf("project name cannot be empty")
					}
				}
				if profile == "" {
					fmt.Print("Auth profile: ")
					profile, _ = reader.ReadString('\n')
					profile = strings.TrimSpace(profile)
					if profile == "" {
						return fmt.Errorf("auth profile cannot be empty")
					}
				}
			}

			// Reject names that would escape the current working directory
			// (e.g. "../evil"), contain path separators ("foo/bar"), or
			// resolve to the cwd itself ("." / "").
			if err := validateProjectName(name); err != nil {
				return err
			}

			// Resolve the target directory: <cwd>/<name>/
			// Config lives at <target>/.wk/wk.toml per ADR-005 Decision 1.
			targetDir := filepath.Join(cwd, name)
			configPath := config.ProjectConfigPath(targetDir)

			// Belt-and-suspenders traversal guard. Even with validateProjectName,
			// require that the cleaned target is an immediate child of cwd —
			// this catches any edge case the name-level check might miss
			// (platform-specific path quirks, future changes, etc.).
			if filepath.Dir(targetDir) != cwd {
				return fmt.Errorf("refusing to scaffold outside current directory: %s", targetDir)
			}

			// Validate profile according to --store-type. File-store profiles
			// live at <target>/.wk/profiles.env (alongside wk.toml per ADR-006
			// Sub-decision 3); keychain profiles live in the user-level
			// profiles.json and must match the active profile.
			var resolvedProfile *auth.Profile
			switch flagStoreType {
			case "", string(auth.StoreKeychain):
				pm := auth.NewProfileManager()
				p, err := pm.GetProfile(profile)
				if err != nil {
					return fmt.Errorf("profile %q not found — run 'wk auth login' first", profile)
				}
				if activeName, err := pm.GetActiveProfile(); err == nil && activeName != profile {
					return fmt.Errorf("active profile %q does not match target profile %q", activeName, profile)
				}
				resolvedProfile = p
			case string(auth.StoreFile):
				fs := auth.NewFileStore(targetDir)
				if !fs.Exists() {
					fmt.Fprintf(os.Stderr,
						"warning: --store-type file specified but no %s found at %s — create one before running commands\n",
						auth.ProfilesEnvFile, fs.Path)
					// resolvedProfile stays nil; snapshot fields will be
					// omitted from wk.toml (omitempty).
				} else {
					p, ferr := fs.GetProfile(profile)
					if ferr != nil {
						return fmt.Errorf("profile %q not found in %s", profile, fs.Path)
					}
					resolvedProfile = p
				}
			default:
				return fmt.Errorf("unknown --store-type %q; valid: %s, %s",
					flagStoreType, auth.StoreKeychain, auth.StoreFile)
			}

			// Check if target already contains a .wk/wk.toml. ADR-005 Decision 2:
			// interactive prompts for overwrite; non-interactive requires --overwrite.
			var existingCfg *config.Config
			if _, err := os.Stat(configPath); err == nil {
				if !flagOverwrite {
					if !interactive {
						return fmt.Errorf("project %q already exists at %s (use --overwrite to replace)", name, targetDir)
					}
					reader := bufio.NewReader(os.Stdin)
					fmt.Fprintf(cmd.OutOrStdout(), "Project config already exists at %s. Overwrite? [y/N]: ", configPath)
					answer, _ := reader.ReadString('\n')
					answer = strings.ToLower(strings.TrimSpace(answer))
					if answer != "y" && answer != "yes" {
						return fmt.Errorf("aborted: existing project at %s not overwritten", configPath)
					}
				}
				// Load the existing config so we can preserve [[sync]] entries
				// across overwrite — init's flags can express at most one entry,
				// and silently discarding multi-entry configs is a data-loss
				// footgun (issue #29). See ADR-005 "Related: Issue #29".
				if cfg, lerr := config.Load(configPath); lerr == nil {
					existingCfg = cfg
				} else {
					fmt.Fprintf(os.Stderr,
						"warning: could not parse existing %s; overwriting from scratch: %v\n",
						configPath, lerr)
				}
			}

			// Create the container directory and .wk/ subdirectory.
			// Decision 2 steps 4–6: scaffold into existing dir or create fresh.
			if err := os.MkdirAll(filepath.Join(targetDir, config.ProjectDir), 0755); err != nil {
				return fmt.Errorf("creating project directory: %w", err)
			}

			// Snapshot workspace/environment/email from the resolved profile
			// into wk.toml per ADR-006 Sub-decision 8. These fields are
			// informational only — runtime routing always uses the profile
			// store. Safe to persist because .wk/ is gitignored (ADR-005).
			// resolvedProfile may be nil in --store-type file deferred mode,
			// in which case omitempty keeps the fields out of wk.toml.
			cfg := &config.Config{
				Name:    name,
				Profile: profile,
			}
			if resolvedProfile != nil {
				cfg.Workspace = resolvedProfile.Workspace
				cfg.WorkspaceID = resolvedProfile.WorkspaceID
				cfg.Environment = resolvedProfile.Environment
				cfg.Email = resolvedProfile.Email
			}

			// Preserve existing sync entries when overwriting. Without this,
			// a developer who hand-edited extra [[sync]] blocks (the current
			// workaround for the single-entry limitation in issue #29) would
			// silently lose them on --overwrite.
			if existingCfg != nil && len(existingCfg.Sync) > 0 {
				cfg.Sync = append(cfg.Sync, existingCfg.Sync...)
				if !flagJSON {
					fmt.Fprintf(os.Stderr,
						"Preserved %d existing [[sync]] entr%s from %s\n",
						len(existingCfg.Sync), pluralY(len(existingCfg.Sync)), configPath)
				}
			}

			if flagServerPath != "" {
				localPath := flagLocalPath
				if localPath == "" {
					localPath = "."
				}

				if flagVerify {
					client, err := resolveVerifyClient(cmd, profile)
					if err != nil {
						return fmt.Errorf("--verify requires auth: %w", err)
					}
					if err := verifyServerPath(cmd, client, flagServerPath); err != nil {
						return err
					}
				}

				// Append unless an identical entry already exists (e.g., when
				// re-running init with the same --server-path for idempotency).
				newEntry := config.SyncEntry{
					ServerPath: flagServerPath,
					LocalPath:  localPath,
				}
				duplicate := false
				for _, existing := range cfg.Sync {
					if existing.ServerPath == newEntry.ServerPath && existing.LocalPath == newEntry.LocalPath {
						duplicate = true
						break
					}
				}
				if !duplicate {
					cfg.Sync = append(cfg.Sync, newEntry)
				}
			}

			if err := config.Save(configPath, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			// Write .wk/.gitignore (self-ignore). ADR-005 Decision 8.
			if err := ensureWkGitignore(targetDir); err != nil {
				return fmt.Errorf("writing .wk/.gitignore: %w", err)
			}

			result := map[string]string{
				"status":  "initialized",
				"name":    name,
				"profile": profile,
				"path":    configPath,
			}

			if flagJSON {
				return rctx.Formatter.Format(cmd.OutOrStdout(), result)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Initialized wk project %q (profile: %s) at %s\n", name, profile, configPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&flagName, "name", "", "Project name (also the container directory name)")
	cmd.Flags().StringVar(&flagInitProfile, "profile", "", "Auth profile name")
	cmd.Flags().StringVar(&flagServerPath, "server-path", "", "Initial sync server path")
	cmd.Flags().StringVar(&flagLocalPath, "local-path", "", "Initial sync local path (defaults to \".\")")
	cmd.Flags().BoolVar(&flagVerify, "verify", false, "Validate server-path exists on Workato before saving")
	cmd.Flags().BoolVar(&flagInitNoInput, "no-input", false, "Force non-interactive mode (fail on missing required flags instead of prompting)")
	cmd.Flags().BoolVar(&flagOverwrite, "overwrite", false, "Overwrite an existing project config without prompting (non-interactive mode)")

	return cmd
}

// validateProjectName rejects project names that would scaffold outside
// the CWD or otherwise break the "project name = container folder name"
// invariant from ADR-005 Decision 1. The name must be a single, ordinary
// path component — no separators, no traversal segments, no leading dots,
// no leading/trailing whitespace.
func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	// Reject leading/trailing whitespace rather than silently trim. A name
	// like " my-project" is invisible in shell output and creates a
	// directory that breaks unquoted scripts (cd my-project fails) — almost
	// always a typo worth surfacing loudly. The prompt for the interactive
	// path already trims the newline via strings.TrimSpace before calling
	// this validator, so clean interactive input still passes.
	if trimmed := strings.TrimSpace(name); trimmed != name {
		return fmt.Errorf("project name %q has leading or trailing whitespace — remove it and retry", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("project name %q must not contain path separators", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("project name %q is invalid", name)
	}
	// A name containing a null byte is never valid on any supported OS
	// and is the classic path-handling footgun — reject explicitly.
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("project name contains a null byte")
	}
	return nil
}

// pluralY returns "y" when n == 1 and "ies" otherwise, for grammatical
// messages like "1 entry" / "3 entries".
func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// wkGitignoreContent is the exact body of <projectRoot>/.wk/.gitignore.
// The "*" + "!.gitignore" idiom hides every tool-managed file from git
// without requiring the developer's project-root .gitignore to list .wk/,
// while keeping the .gitignore itself visible and committable. See
// ADR-005 Decision 8.
const wkGitignoreContent = `# wk CLI — tool-managed directory. Contents are machine-local state
# (project config, sidecar metadata, folder-ID cache). Do not commit.
*
!.gitignore
`

// ensureWkGitignore writes <projectRoot>/.wk/.gitignore with the fixed
// self-ignore content. The file is owned by the CLI — overwriting it
// unconditionally is intentional so developers can rely on the content
// never drifting. The project-root .gitignore is never touched: if the
// developer maintains one, it remains their file.
func ensureWkGitignore(projectRoot string) error {
	wkDir := filepath.Join(projectRoot, config.ProjectDir)
	if err := os.MkdirAll(wkDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", wkDir, err)
	}
	path := filepath.Join(wkDir, ".gitignore")
	return os.WriteFile(path, []byte(wkGitignoreContent), 0644)
}
