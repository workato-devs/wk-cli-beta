package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/api"
	"github.com/workato-devs/wk-cli-beta/internal/auth"
	"github.com/workato-devs/wk-cli-beta/internal/config"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication profiles",
	}
	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthStatusCmd())
	cmd.AddCommand(newAuthSwitchCmd())
	cmd.AddCommand(newAuthListCmd())
	cmd.AddCommand(newAuthDeleteCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var (
		name        string
		workspace   string
		environment string
		region      string
		token       string
		force       bool
		noInput     bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Create or update an auth profile",
		Long: `Create or update an auth profile.

The CLI introspects the workspace from GET /users/me, so --workspace is an
optional override. The profile name is auto-computed from the workspace and
environment when --name is omitted.

Non-interactive mode (detected via --json, --no-input, or a non-TTY stdin)
requires --token and --environment explicitly. See ADR-006.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			interactive := isInteractiveStdin() && !noInput && !flagJSON
			reader := bufio.NewReader(os.Stdin)

			// In non-interactive mode, validate every required flag upfront
			// before any API call or prompt. This prevents prompt labels
			// from ever reaching the terminal in batch contexts.
			if !interactive {
				var missing []string
				if token == "" {
					missing = append(missing, "--token")
				}
				if environment == "" {
					missing = append(missing, "--environment")
				}
				if len(missing) > 0 {
					return fmt.Errorf("%s required in non-interactive mode (detected via --json, --no-input, or non-TTY stdin)",
						strings.Join(missing, " and "))
				}
			}

			// Step 1: token. Prompt only in interactive mode.
			if token == "" {
				fmt.Print("API token: ")
				line, _ := reader.ReadString('\n')
				token = strings.TrimSpace(line)
			}
			if token == "" {
				return fmt.Errorf("API token is required")
			}

			// Step 1b: region. Defaults silently to "us".
			if region == "" {
				region = config.DefaultRegion
			}
			r := auth.Region(region)
			if !r.IsValid() {
				regions := auth.ValidRegions()
				names := make([]string, len(regions))
				for i, rg := range regions {
					names[i] = string(rg)
				}
				return fmt.Errorf("invalid region %q; valid regions: %s", region, strings.Join(names, ", "))
			}
			baseURL := config.BaseURL(region)

			// Step 2: GET /users/me. Populates workspace, workspace_id, email.
			// Failure here aborts login — a token that can't authenticate is
			// not worth persisting.
			tempClient := api.NewHTTPClient(baseURL+config.APIPathPrefix, token)
			info, err := tempClient.Workspace().GetCurrentWorkspace(cmd.Context())
			if err != nil {
				return fmt.Errorf("validating token via /users/me: %w", err)
			}

			// Step 3: --workspace mismatch check.
			if workspace != "" && workspace != info.Name {
				return fmt.Errorf("--workspace %q does not match the token's workspace %q; the API's value is authoritative",
					workspace, info.Name)
			}
			workspace = info.Name

			// Step 4: environment. Only prompted in interactive mode;
			// non-interactive mode was already validated upfront.
			if environment == "" {
				fmt.Print("Environment (e.g. dev, staging, prod): ")
				line, _ := reader.ReadString('\n')
				environment = strings.TrimSpace(line)
			}
			if environment == "" {
				return fmt.Errorf("environment cannot be empty")
			}

			// Step 5: name. Auto-compute <region>-<workspace-slug>-<env>
			// when --name is omitted; in interactive mode, show the computed
			// default in the prompt.
			computed := computeProfileName(workspace, environment, region)
			if name == "" {
				if interactive {
					fmt.Printf("Profile name [%s]: ", computed)
					line, _ := reader.ReadString('\n')
					name = strings.TrimSpace(line)
				}
				if name == "" {
					name = computed
				}
			}

			pm := auth.NewProfileManager()

			// Overwrite detection: check if profile name already exists.
			if existing, _ := pm.GetProfile(name); existing != nil && !force {
				if !interactive {
					return fmt.Errorf("profile %q already exists — use --force to overwrite", name)
				}
				fmt.Fprintf(os.Stderr, "Profile %q already exists (workspace: %s, environment: %s, region: %s)\n",
					existing.Name, existing.Workspace, existing.Environment, existing.Region)
				fmt.Print("Overwrite? [y/N]: ")
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(strings.ToLower(answer))
				if answer != "y" && answer != "yes" {
					fmt.Fprintln(os.Stderr, "Aborted.")
					return nil
				}
			}

			now := time.Now()

			profile := &auth.Profile{
				Name:        name,
				Workspace:   info.Name,
				WorkspaceID: info.ID,
				Environment: environment,
				Email:       info.Email,
				Region:      r,
				StoreType:   auth.StoreKeychain,
				BaseURL:     baseURL,
				CreatedAt:   now,
			}

			cred := &auth.Credential{
				Token:     token,
				Region:    r,
				StoreType: auth.StoreKeychain,
				CreatedAt: now,
			}

			if err := pm.SaveProfile(profile); err != nil {
				return fmt.Errorf("saving profile: %w", err)
			}

			store := &auth.KeyringStore{}
			if err := store.Set(cmd.Context(), name, cred); err != nil {
				return fmt.Errorf("storing credential: %w", err)
			}

			if err := pm.SetActiveProfile(name); err != nil {
				return fmt.Errorf("setting active profile: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Profile %q saved and set as active (workspace: %s, environment: %s, region: %s)\n",
				name, workspace, environment, region)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Profile name (default: <region>-<workspace-slug>-<environment>)")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Override workspace; must match the token's workspace (default: introspected from /users/me)")
	cmd.Flags().StringVar(&environment, "environment", "", "Target environment (e.g. dev, staging, prod); required in non-interactive mode")
	cmd.Flags().StringVar(&region, "region", config.DefaultRegion, "Workato region (us, eu, jp, au, sg, il, cn, trial (Developer Sandbox))")
	cmd.Flags().StringVar(&token, "token", "", "Workato API token; required in non-interactive mode")
	cmd.Flags().BoolVar(&force, "force", false, "Skip overwrite confirmation if profile already exists")
	cmd.Flags().BoolVar(&noInput, "no-input", false, "Force non-interactive mode (fail on missing required flags instead of prompting)")
	return cmd
}

// isInteractiveStdin reports whether the CLI can meaningfully prompt the
// user — it requires BOTH stdin and stdout to be attached to a terminal.
// Checking stdout too catches cases where output is captured to a file or
// piped further, in which case prompt labels become noise (the user can't
// read them inline with their own input).
func isInteractiveStdin() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// computeProfileName returns <region>-<workspace-slug>-<environment> per
// ADR-006. Region is always the leading component so that profiles sort
// and group by region in `wk auth list`, and so the most discriminating
// field is immediately visible rather than hidden behind slug collisions.
// Empty region falls back to config.DefaultRegion ("us").
func computeProfileName(workspace, environment, region string) string {
	r := region
	if r == "" {
		r = config.DefaultRegion
	}
	return r + "-" + slugify(workspace) + "-" + environment
}

// slugify lowercases s, replaces non-alphanumerics with "-", collapses
// repeated separators, and trims leading/trailing separators.
func slugify(s string) string {
	var b strings.Builder
	lastSep := true // suppress a leading separator
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastSep = false
			continue
		}
		if !lastSep {
			b.WriteRune('-')
			lastSep = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show active profile and test connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			pm := auth.NewProfileManager()
			activeName, err := pm.GetActiveProfile()
			if err != nil {
				return fmt.Errorf("no active profile: %w", err)
			}

			profile, _, credErr := resolveProfileAndCred(cmd.Context(), activeName)
			if profile == nil {
				// Fall back to metadata-only read so status can still report
				// which profile is configured even if the backend can't be
				// reached.
				p, perr := pm.GetProfile(activeName)
				if perr != nil {
					return fmt.Errorf("profile %q: %w", activeName, perr)
				}
				profile = p
			}

			type statusInfo struct {
				Profile     string `json:"profile"`
				Workspace   string `json:"workspace"`
				Environment string `json:"environment"`
				Region      string `json:"region"`
				BaseURL     string `json:"base_url"`
				HasCreds    bool   `json:"has_credentials"`
				StoreType   string `json:"store_type"`
				Connected   bool   `json:"connected"`
				ConnError   string `json:"conn_error,omitempty"`
			}
			info := statusInfo{
				Profile:     profile.Name,
				Workspace:   profile.Workspace,
				Environment: profile.Environment,
				Region:      string(profile.Region),
				BaseURL:     profile.BaseURL,
				HasCreds:    credErr == nil,
				StoreType:   string(profile.StoreType),
			}

			if credErr == nil {
				client, _, clientErr := resolveAPIClient(cmd)
				if clientErr == nil {
					ctx := cmd.Context()
					_, apiErr := client.Recipes().List(ctx, &api.RecipeListOptions{PerPage: 1})
					if apiErr == nil {
						info.Connected = true
					} else {
						info.ConnError = apiErr.Error()
					}
				} else {
					info.ConnError = clientErr.Error()
				}
			}

			if !flagJSON {
				hasCreds := "no"
				if info.HasCreds {
					hasCreds = "yes"
				}
				fmt.Fprintf(os.Stdout, "Profile:     %s\n", info.Profile)
				fmt.Fprintf(os.Stdout, "Workspace:   %s\n", info.Workspace)
				fmt.Fprintf(os.Stdout, "Environment: %s\n", info.Environment)
				fmt.Fprintf(os.Stdout, "Region:      %s\n", info.Region)
				fmt.Fprintf(os.Stdout, "Base URL:    %s\n", info.BaseURL)
				fmt.Fprintf(os.Stdout, "Credentials: %s\n", hasCreds)
				fmt.Fprintf(os.Stdout, "Store:       %s\n", info.StoreType)
				if info.Connected {
					fmt.Fprintf(os.Stdout, "API:         connected\n")
				} else if info.ConnError != "" {
					fmt.Fprintf(os.Stdout, "API:         %s\n", info.ConnError)
				}

				if profile.Workspace == "" || profile.Environment == "" {
					fmt.Fprintf(os.Stderr, "\nWarning: profile missing workspace/environment — run 'wk auth login' to update\n")
				}
				return nil
			}
			return rctx.Formatter.Format(os.Stdout, info)
		},
	}
}

func newAuthSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <name>",
		Short: "Switch active profile",
		Args:  requireArgs(1, "profile name is required, e.g.: wk auth switch <name>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			pm := auth.NewProfileManager()

			if _, err := pm.GetProfile(name); err != nil {
				// Keychain miss: check if the name exists in profiles.env to
				// give a more useful error. File-store profiles are project-
				// scoped and cannot be set as the global active profile.
				if cwd, werr := os.Getwd(); werr == nil {
					if root, rerr := config.FindProjectRoot(cwd); rerr == nil {
						fs := auth.NewFileStore(root)
						if fs.Exists() {
							if _, fperr := fs.GetProfile(name); fperr == nil {
								return fmt.Errorf(
									"profile %q is a file-store profile; file-store profiles are resolved per-invocation via --profile %s --store-type file and cannot be set as the global active profile",
									name, name)
							}
						}
					}
				}
				return fmt.Errorf("profile %q not found", name)
			}

			if err := pm.SetActiveProfile(name); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Switched to profile %q\n", name)
			return nil
		},
	}
}

func newAuthListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all auth profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			pm := auth.NewProfileManager()
			profiles, err := pm.ListProfiles()
			if err != nil {
				return err
			}

			activeName, _ := pm.GetActiveProfile()
			keychainNames := make(map[string]bool, len(profiles))
			for _, p := range profiles {
				keychainNames[p.Name] = true
			}

			headers := []string{"NAME", "WORKSPACE", "ENVIRONMENT", "REGION", "STORE", "ACTIVE"}
			var rows [][]string
			for _, p := range profiles {
				rows = append(rows, profileRow(p, activeName == p.Name, false))
			}

			// Merge file-store profiles when inside a project with profiles.env.
			if cwd, werr := os.Getwd(); werr == nil {
				if root, rerr := config.FindProjectRoot(cwd); rerr == nil {
					fs := auth.NewFileStore(root)
					if fs.Exists() {
						fileProfiles, ferr := fs.ListProfiles()
						if ferr != nil {
							fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", fs.Path, ferr)
						}
						for _, p := range fileProfiles {
							rows = append(rows, profileRow(p, false, keychainNames[p.Name]))
						}
					}
				}
			}

			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}
}

// profileRow formats one auth-list row. When shadowed is true, the name is
// annotated to show that keychain routing would shadow this file-store entry.
func profileRow(p *auth.Profile, isActive, shadowed bool) []string {
	active := ""
	if isActive {
		active = "*"
	}
	name := p.Name
	if shadowed {
		name += " (shadowed)"
	}
	ws := p.Workspace
	if ws == "" {
		ws = "(unset)"
	}
	env := p.Environment
	if env == "" {
		env = "(unset)"
	}
	return []string{name, ws, env, string(p.Region), string(p.StoreType), active}
}

func newAuthDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an auth profile and its stored credential",
		Args:  requireArgs(1, "profile name is required, e.g.: wk auth delete <name>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			pm := auth.NewProfileManager()

			profile, err := pm.GetProfile(name)
			if err != nil {
				return fmt.Errorf("profile %q not found", name)
			}

			// Remove credential from keyring.
			store := &auth.KeyringStore{}
			if err := store.Delete(cmd.Context(), name); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not remove credential from keyring: %v\n", err)
			}

			// Remove profile metadata.
			if err := pm.DeleteProfile(name); err != nil {
				return fmt.Errorf("deleting profile: %w", err)
			}

			// If this was the active profile, clear it.
			if activeName, _ := pm.GetActiveProfile(); activeName == name {
				_ = pm.SetActiveProfile("")
			}

			fmt.Fprintf(os.Stdout, "Deleted profile %q (workspace: %s, environment: %s)\n",
				profile.Name, profile.Workspace, profile.Environment)
			return nil
		},
	}
}
