# ADR-006: Profile Identity Model — Workspace & Environment Metadata

**Status:** Proposed
**Date:** April 9, 2026
**Authors:** Zayne Turner, Chris Miller
**Deciders:** DevRel Engineering, Platform CLI Team
**References:** ADR-001 (Decision 4: Project Model, Decision 5: Credential Storage)

---

## Context

The original profile model (ADR-001) used a single `--name` flag as both the profile identifier and the implicit workspace reference. The `Profile` struct contained `name`, `region`, `store_type`, and `base_url`. The `wk.toml` project config had a `workspace` field that stored a profile name, and `checkWorkspaceMatch` compared the active profile name against it.

This conflated three distinct concepts:

1. **Identity** — how the CLI looks up a profile (the `name` field)
2. **Targeting** — which Workato account and environment the profile points at
3. **Project binding** — which profile a project is pinned to

As multi-workspace and multi-environment workflows become standard, the CLI needs explicit `workspace` and `environment` fields on profiles. Without them, a developer managing profiles for `acme-corp/dev`, `acme-corp/prod`, and `partner-inc/dev` has no structured way to distinguish what each profile targets — only the name they happened to choose.

Additionally, the `wk.toml` field `workspace` stored a profile name but was named after a concept that now has its own distinct meaning, creating ambiguity in documentation, help text, and developer mental models.

---

## Decision

### Profile struct expansion

Add `workspace` (required) and `environment` (required) as metadata fields on the `Profile` struct. The `name` field remains the primary key — it is required, developer-chosen, and is what the CLI uses for lookup, keyring storage, active profile tracking, and project binding.

**Before:**
```go
type Profile struct {
    Name      string    `json:"name"`
    Region    Region    `json:"region"`
    StoreType StoreType `json:"store_type"`
    BaseURL   string    `json:"base_url"`
    CreatedAt time.Time `json:"created_at"`
}
```

**After:**
```go
type Profile struct {
    Name        string    `json:"name"`
    Workspace   string    `json:"workspace"`
    Environment string    `json:"environment"`
    Region      Region    `json:"region"`
    StoreType   StoreType `json:"store_type"`
    BaseURL     string    `json:"base_url"`
    CreatedAt   time.Time `json:"created_at"`
}
```

### Login command flags

`wk auth login` requires three flags:
- `--name` — developer-chosen profile identifier (primary key)
- `--workspace` — Workato account name (required)
- `--environment` — target environment within the workspace (required)
- `--token` — API token (required, unchanged)
- `--region` — Workato region (unchanged, defaults to `us`)

### wk.toml field rename

The `wk.toml` field that pins a project to a profile is renamed from `workspace` to `profile`, since "workspace" now has a distinct meaning (the Workato account).

**Before:**
```toml
name = "my-project"
workspace = "dev"
```

**After:**
```toml
name = "my-project"
profile = "dev"
```

The Go struct field is renamed from `Config.Workspace` to `Config.Profile` with the TOML tag `toml:"profile"`.

### What does NOT change

- The `CredentialStore` interface — still keyed by a single `profileName string`
- Keyring storage keys — still the profile `name`
- `~/.wk/active_profile` — still a single name string
- `~/.wk/keyring_profiles.json` — still a string list of names
- The `--profile` global flag — still accepts a profile name
- `auth switch <name>` — still a single argument

---

## Sub-Decisions

### 1. Workspace + Environment uniqueness

The CLI enforces that no two profiles share the same `(workspace, environment, region)` tuple. This prevents silent misconfiguration where two profiles accidentally target the same remote environment. The `name` field remains independently unique (as it is the primary key).

### 2. Environment field validation

The `environment` field is a freeform string, not a constrained enum. Workato's environment model varies by account tier and configuration, so the CLI should not impose a fixed set. Examples: `dev`, `staging`, `prod`, `sandbox`, `test`.

### 3. Env store mapping

When credentials come from environment variables (`WK_TOKEN`), the `workspace` and `environment` fields are populated from `WK_WORKSPACE` and `WK_ENVIRONMENT` env vars if set. If not set, the synthetic env profile has empty workspace/environment fields. This maintains backward compatibility for CI/CD pipelines that only set `WK_TOKEN` and `WK_REGION`.

### 4. Clone command flag rename

The `clone` command's `--workspace` flag (which specifies a server-side path prefix, not an auth workspace) is renamed to `--path-prefix` to avoid collision with the auth concept of workspace.

### 5. Backward compatibility

Old `profiles.json` entries without `workspace` or `environment` fields deserialize with empty strings (Go zero values). The CLI warns on `auth status` and `auth list` when a profile has empty workspace/environment fields and suggests running `wk auth login` to update it. No blocking migration — existing profiles continue to function.

### 6. Terminology alignment

All CLI help text, error messages, prompts, and code comments are updated to use consistent terminology:
- **Profile** = a named auth configuration (workspace + environment + region + credential)
- **Workspace** = a Workato account
- **Environment** = a target within a workspace

The phrase "workspace profile" is eliminated. References to "workspace" in contexts that mean "profile" are corrected.

---

## Alternatives Considered

### Composite key (workspace + environment as the identifier)

Using `workspace/environment` as the profile lookup key instead of a separate `name`. This was rejected because:
- It forces a breaking change to every layer that passes a profile identifier (keyring, active_profile file, wk.toml, CredentialStore interface)
- It makes the `auth switch` command verbose (`wk auth switch acme-corp/dev` vs. `wk auth switch dev`)
- It requires migration of existing keychain entries and config files
- The developer-chosen `name` provides a convenient short alias that can be anything

### Optional workspace and environment

Making the new fields optional rather than required. This was rejected because optional fields would allow profiles to exist without targeting information, defeating the purpose of the change. Existing profiles are handled via backward compatibility (empty strings with a warning), but new profiles must provide both fields.

---

## Consequences

### What becomes easier

- **Multi-environment workflows**: Developers can see at a glance which workspace and environment each profile targets, rather than relying on naming conventions.
- **Automation and scripting**: CI/CD pipelines can set `WK_WORKSPACE` and `WK_ENVIRONMENT` for auditability, making it clear which environment a pipeline run targeted.
- **Safety**: `auth list` output shows the actual target, reducing the risk of operating against the wrong environment because a profile was misleadingly named.

### What becomes harder

- **Profile creation**: `wk auth login` now requires two additional flags. Mitigated by the interactive prompt flow filling them in when omitted.
- **Existing users**: Profiles created before this change lack workspace/environment fields. Mitigated by the non-blocking backward compatibility strategy (warn, don't block).

### What we'll need to revisit

- **Profile validation against Workato API**: Once the API supports workspace/environment introspection, the CLI could validate that the workspace and environment specified in a profile actually exist on the server. Deferred until API support is available.
- **wk.toml binding granularity**: Currently the project pins to a profile name. A future enhancement could pin to a workspace (allowing any profile targeting that workspace), enabling environment promotion workflows without changing wk.toml.

---

## Action Items

1. [ ] Update `Profile` struct in `internal/auth/types.go` with `Workspace` and `Environment` fields
2. [ ] Add `--workspace` and `--environment` required flags to `wk auth login`
3. [ ] Rename `Config.Workspace` to `Config.Profile` (struct + TOML tag)
4. [ ] Rename `clone --workspace` to `clone --path-prefix`
5. [ ] Update all help text, error messages, and prompts for terminology consistency
6. [ ] Add `WK_WORKSPACE` and `WK_ENVIRONMENT` env var support to `EnvStore`
7. [ ] Add workspace+environment+region uniqueness validation to `ProfileManager.SaveProfile()`
8. [ ] Update `auth list` table columns to include WORKSPACE and ENVIRONMENT
9. [ ] Update all tests
