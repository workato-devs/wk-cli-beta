# ADR-006: Profile Identity Model — Workspace & Environment Metadata

**Status:** Proposed
**Date:** April 9, 2026
**Last Revised:** April 17, 2026
**Authors:** Zayne Turner, Chris Miller
**Deciders:** DevRel Engineering, Platform CLI Team
**References:** ADR-001 (Decision 4: Project Model, Decision 5: Credential Storage), ADR-005 (Decision 8: `.wk/` gitignored)

> **Revision history**
> - **April 17, 2026** — Integrated login ergonomic improvements from field testing: `--workspace` becomes an optional override (introspected from `GET /users/me`), `--name` gains a computed default from `<workspace-slug>-<environment>[-<region>]`, and `wk.toml` is extended with `workspace`, `workspace_id`, `environment`, and `email` as informational snapshot fields (safe because `.wk/` is gitignored per ADR-005). Non-interactive mode behavior formalized as Sub-decision 10. Environment introspection via `/activity_logs` considered and deferred.

---

## Context

The original profile model (ADR-001) used a single `--name` flag as both the profile identifier and the implicit workspace reference. The `Profile` struct contained `name`, `region`, `store_type`, and `base_url`. The `wk.toml` project config had a `workspace` field that stored a profile name, and `checkWorkspaceMatch` compared the active profile name against it.

This conflated three distinct concepts:

1. **Identity** — how the CLI looks up a profile (the `name` field)
2. **Targeting** — which Workato account and environment the profile points at
3. **Project binding** — which profile a project is pinned to

As multi-workspace and multi-environment workflows become standard, the CLI needs explicit `workspace` and `environment` fields on profiles. Without them, a developer managing profiles for `acme-corp/dev`, `acme-corp/prod`, and `partner-inc/dev` has no structured way to distinguish what each profile targets — only the name they happened to choose.

Additionally, the `wk.toml` field `workspace` stored a profile name but was named after a concept that now has its own distinct meaning, creating ambiguity in documentation, help text, and developer mental models.

**Follow-up observations (April 17, 2026 revision):** Field testing of the initial login flow surfaced two further friction points. First, the five required prompts for a first-time `auth login` (name, workspace, environment, region, token) were rated awkward — particularly because investigation of Workato's public API showed that `GET /users/me` returns the workspace `id` and `name` directly (the local client struct captured the fields via matching JSON tags but the login flow never called the endpoint). Second, storing only the profile name in `wk.toml` made it hard to tell at a glance which workspace, environment, and account a project is bound to when juggling several profiles with short aliases. The decisions below are revised to address both, with the additional observation that `wk.toml` is local-only (per ADR-005 Decision 8, `.wk/` is gitignored) — so informational snapshot fields can be persisted safely without PII or cross-developer drift concerns.

---

## Decision

### Profile struct expansion

Add `workspace`, `workspace_id`, `environment`, and `email` as metadata fields on the `Profile` struct. The `name` field remains the primary key — it is required, developer-chosen, and is what the CLI uses for lookup, keyring storage, active profile tracking, and project binding. `workspace`, `workspace_id`, and `email` are populated from the `GET /users/me` response at login time; `environment` is user-provided.

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
    WorkspaceID int       `json:"workspace_id"`
    Environment string    `json:"environment"`
    Email       string    `json:"email"`
    Region      Region    `json:"region"`
    StoreType   StoreType `json:"store_type"`
    BaseURL     string    `json:"base_url"`
    CreatedAt   time.Time `json:"created_at"`
}
```

The three API-sourced fields (`Workspace`, `WorkspaceID`, `Email`) are populated exclusively from `/users/me` during login — the `Profile` is the single place they live. `wk init` reads them back from the resolved profile to populate the `wk.toml` informational snapshot (see the `wk.toml` section below); there is no second `/users/me` call at init time.

This schema is universal — every profile carries these fields regardless of credential store type (keychain, file, encrypted file, secrets manager). The store type determines where the profile metadata and credential are persisted, not what fields the profile contains. See Sub-decision 9 for the full four-tier storage pattern.

### Login command flags

Every required profile field on `wk auth login` follows the same resolution order: **flag → default → prompt (interactive only)**. What varies is the source of the default.

| Flag | Required on profile? | Default source |
|---|---|---|
| `--token` | Yes | None — user must provide |
| `--region` | Yes | Static (`us`) |
| `--name` | Yes | Computed: `<region>-<workspace-slug>-<environment>` (see below) |
| `--workspace` | Yes | Introspected from `GET /users/me` immediately after token validation |
| `--environment` | Yes | Prompted interactively; **required explicitly in non-interactive mode** (see Sub-decision 10) |
| `--store-type` | No | Defaults to `keychain` (see Sub-decision 3) |
| `--force` | No | Off; skips the overwrite-confirmation prompt |

`--workspace` is an optional override. When provided and non-empty, it must match the workspace the token authenticates against, or the CLI errors — the API's value is authoritative. When omitted, the introspected value from `/users/me` is used.

`--environment` remains user-provided in this revision. Introspection via `GET /activity_logs.workspace.environment` was considered but deferred due to edge cases (empty logs on new workspaces, missing audit permissions, network failures) that require careful fallback design.

**Auto-name default format:** When `--name` is omitted, the CLI computes `<region>-<workspace-slug>-<environment>`:

- Region is always the leading component — including for `us`. This makes the most discriminating field immediately visible, groups profiles by region in `wk auth list` output, and avoids the prior "region hidden in common case, tacked on the end otherwise" inconsistency that testing surfaced as confusing.
- Slugify: lowercase the workspace name, replace non-alphanumerics with `-`, collapse repeated separators, trim.
- Empty region falls back to `us` (the default).

Examples:

- `"Acme Corp"` + `prod` + `us` → `us-acme-corp-prod`
- `"Acme Corp"` + `prod` + `eu` → `eu-acme-corp-prod`
- `"Acme"` + `dev` + `il` → `il-acme-dev`

In interactive mode, the computed default is shown in the prompt (`Profile name [us-acme-corp-prod]:`) and the developer can edit it. In non-interactive mode, the default is applied silently when `--name` is omitted. Collisions rely on the existing uniqueness constraints (Sub-decision 1): a re-login producing an already-taken name errors from `SaveProfile`, and the developer either passes `--force` or supplies an explicit `--name`.

**Interactive prompt order:** Introspection requires the token before workspace/email are known, and the auto-computed name requires workspace + environment before its default is meaningful. The interactive flow resolves fields in this order:

1. **Token** — prompt if `--token` is not provided. Required before any API call.
2. **`GET /users/me`** — called silently with the token. Populates the in-memory profile's `workspace`, `workspace_id`, and `email`. Failure here aborts login (a token that can't authenticate is not worth persisting).
3. **`--workspace` mismatch check** — if `--workspace` was supplied explicitly, compare to the introspected value and error on mismatch.
4. **Environment** — prompt if `--environment` is not provided.
5. **Name** — prompt if `--name` is not provided, showing the auto-computed default in brackets. Computed default is `<region>-<workspace-slug>-<environment>`.
6. **Region** — accepts the `us` default silently; prompted only if `--region` is explicitly set to an empty/invalid value or if a future revision adds a region-selection flow.
7. **Save** — uniqueness checks (Sub-decision 1), overwrite-confirmation if applicable, persist to the credential store.

Non-interactive mode follows the same steps but replaces each prompt with a flag-lookup that hard-fails when the required flag is absent (Sub-decision 10). Step 2 (`/users/me`) runs identically in both modes.

**Overwrite behavior:** If a profile with the given `--name` already exists, the CLI warns and prompts for confirmation before overwriting. The `--force` flag acknowledges the overwrite in advance, skipping the prompt. Developers who prefer explicit separation can delete the existing profile first (`wk auth delete <name>`) and re-create it.

### wk.toml field rename and informational snapshot

The `wk.toml` field that pins a project to a profile is renamed from `workspace` to `profile`, since "workspace" now has a distinct meaning (the Workato account). `wk.toml` is additionally extended with a snapshot of the bound profile's workspace, environment, and authenticated email so developers can tell at a glance which account a project is targeting.

**Before:**
```toml
name = "my-project"
workspace = "dev"
```

**After:**
```toml
name = "my-project"
profile = "dev"
workspace = "Acme Corp"
workspace_id = 12345
environment = "prod"
email = "zayne@workato.com"
```

The Go struct field is renamed from `Config.Workspace` to `Config.Profile` (TOML tag `toml:"profile"`). New fields `Workspace` (string), `WorkspaceID` (int), `Environment` (string), and `Email` (string) are added with matching TOML tags.

The snapshot fields are **informational only** — runtime routing and credential resolution always use the profile's authoritative store. They are written at `init` time from the resolved profile. Persisting them in `wk.toml` is safe because the entire `.wk/` directory is gitignored per ADR-005 Decision 8, so no PII or environment-specific data reaches version control. See Sub-decision 8 for the full storage model.

### What does NOT change

- The `CredentialStore` interface — still keyed by a single `profileName string`
- Keyring storage keys — still the profile `name`
- `~/.wk/active_profile` — still a single name string
- `~/.wk/keyring_profiles.json` — still a string list of names
- The `--profile` global flag — still accepts a profile name
- `auth switch <name>` — still a single argument

### What DOES change

- **Credential routing model** — The `ChainStore` pattern (try env vars, then keychain in fixed order) is replaced by StoreType-driven routing. The profile's `store_type` field determines which credential backend to query. See Sub-decision 6.
- **Environment variable credential support** — Process environment variables (`WK_TOKEN`, `WK_REGION`) are removed. The CLI no longer reads credentials from the process environment. CI/CD pipelines use the project-level file store instead. See Sub-decision 7.
- **`auth login` store type** — No longer hardcoded to `StoreKeychain`. The `--store-type` flag selects the credential backend at profile creation time.
- **`auth list` output** — Adds WORKSPACE and ENVIRONMENT columns. Shows file-store profiles from `profiles.env` when run inside a project directory.
- **`wk init` validation** — Now validates that the named profile exists and that the active profile matches. See Sub-decision 8.

---

## Sub-Decisions

### 1. Workspace + Environment uniqueness

The CLI enforces that no two profiles share the same `(workspace, environment, region)` tuple. This prevents silent misconfiguration where two profiles accidentally target the same remote environment. The `name` field remains independently unique (as it is the primary key).

### 2. Environment field validation

The `environment` field is a freeform string, not a constrained enum. Workato's environment model varies by account tier and configuration, so the CLI should not impose a fixed set. Examples: `dev`, `staging`, `prod`, `sandbox`, `test`.

### 3. File-based credential store (`profiles.env`)

The file-based credential store replaces the previous process-environment-variable approach (`WK_TOKEN`, `WK_REGION`). Profile metadata and credentials are stored together in a project-level `profiles.env` file using key=value syntax.

**Format:**

```
NAME=dev
WORKSPACE=acme-corp
ENVIRONMENT=dev
REGION=us
STORE_TYPE=file
BASE_URL=https://www.workato.com
TOKEN=wk-xxxxx

NAME=prod
WORKSPACE=acme-corp
ENVIRONMENT=prod
REGION=us
STORE_TYPE=file
BASE_URL=https://www.workato.com
TOKEN=wk-yyyyy
```

- Each `NAME=` line starts a new profile record. Fields following it until the next `NAME=` line (or EOF) belong to that profile.
- Keys are unprefixed — no `WK_` prefix. The file is CLI-owned; key names correspond directly to profile struct fields.
- The CLI reads but **does not write** to this file. Developers and CI/CD pipelines create and manage it directly.
- **Path:** `<project-root>/.wk/profiles.env` — alongside `wk.toml` inside the tool-managed directory (per ADR-005 Decision 1). Co-locating with `wk.toml` also means `profiles.env` is automatically covered by `.wk/.gitignore` (ADR-005 Decision 8), so the secrets file cannot be committed by accident.
- Multi-profile files are supported (designed for) but single-profile is the common case.
- A `docs/profiles.env.example` file in the repo shows the expected format. It deliberately lives under `docs/` rather than the project root so that a developer can't confuse it for a working `profiles.env` in the wrong location — the real file belongs at `<your-project>/.wk/profiles.env`.

**Note:** This is not a standard `.env` file — standard dotenv parsers expect unique keys. This is a CLI-owned file that uses key=value syntax with `NAME=` as a record delimiter. The `.env` extension signals "key=value config with secrets — do not commit," which is the right developer signal.

### 4. Clone command flag rename

The `clone` command's `--workspace` flag (which specifies a server-side path prefix, not an auth workspace) is renamed to `--path-prefix` to avoid collision with the auth concept of workspace.

### 5. Terminology alignment

All CLI help text, error messages, prompts, and code comments are updated to use consistent terminology:
- **Profile** = a named auth configuration (workspace + environment + region + credential)
- **Workspace** = a Workato account
- **Environment** = a target within a workspace

The phrase "workspace profile" is eliminated. References to "workspace" in contexts that mean "profile" are corrected.

### 6. StoreType-driven credential routing

The `ChainStore` pattern — which tries env vars then keychain in a fixed, hardcoded order — is replaced by deterministic routing based on the profile's `store_type` field.

**Resolution flow:**

1. Read active profile name from `~/.wk/active_profile` (or `--profile` flag override)
2. Look up profile metadata:
   a. Check `~/.wk/profiles.json` — if found, the profile's `store_type` field determines the credential backend
   b. If not in `profiles.json`, check project-level `profiles.env` — if found, store type is implicitly `file`
   c. If neither, error: profile not found
3. Retrieve credential from the backend indicated by the resolved store type

The `--store-type` global flag can override implicit routing, allowing a developer to explicitly target a specific backend (e.g., `wk pull --profile dev --store-type file` routes to `profiles.env` even if `dev` also exists in `profiles.json`).

**Cross-store name collisions:** If the same profile name exists in both `profiles.json` and `profiles.env`, the lookup order (step 2) provides deterministic priority: keychain wins. Developers can override with `--store-type file`. No collision enforcement is needed — the priority order and explicit flag handle it.

### 7. CI/CD model

Process environment variables (`WK_TOKEN`, `WK_REGION`) are removed. CI/CD pipelines use the project-level file store:

1. The pipeline generates `profiles.env` from its secrets management system (GitHub Secrets, Vault, etc.)
2. The project's committed `wk.toml` references a profile name (e.g., `profile = "ci"`)
3. CLI commands resolve credentials from `profiles.env` at runtime — no `wk auth login` step is needed

This provides consistency across local and CI/CD environments: the same profile schema, the same lookup-by-name behavior. The only difference is who creates the credential file (developer vs. pipeline script).

**Example (GitHub Actions):**

```yaml
steps:
  - uses: actions/checkout@v4
  - name: Write credentials
    run: |
      mkdir -p .wk
      cat <<EOF > .wk/profiles.env
      NAME=ci
      WORKSPACE=acme-corp
      ENVIRONMENT=prod
      REGION=us
      STORE_TYPE=file
      BASE_URL=https://www.workato.com
      TOKEN=${{ secrets.WORKATO_TOKEN }}
      EOF
  - name: Pull recipes
    run: wk pull --profile ci
```

### 8. `wk init` profile validation

`wk init` is updated to validate the named profile and enforce active-profile consistency. These changes are cohesive with ADR-005 (project scaffolding).

**Profile validation:** `init --profile <name>` validates that the named profile exists before writing `wk.toml`. The `--store-type` flag tells `init` which store to check:

- Default (no `--store-type`): checks `~/.wk/profiles.json` (keychain profiles)
- `--store-type file`: checks `<target>/.wk/profiles.env`. Per ADR-005 Decision 2 step 4, `init` can scaffold into an existing directory — the developer creates `<target>/.wk/` and places `profiles.env` in it before running `init` (a one-liner: `mkdir -p my-project/.wk && write-secrets > my-project/.wk/profiles.env`).

If `--store-type file` is specified and no `profiles.env` exists at that path, `init` warns ("no profiles.env found — create one before running commands") and defers credential validation to runtime. Profile metadata is not validated in this case.

**Active profile mismatch:** If the active profile (from `~/.wk/active_profile`) does not match the `--profile` argument, `init` fails:

```
Error: active profile "prod" does not match target profile "dev"
```

No corrective action is suggested — the developer resolves the mismatch themselves.

**What `wk.toml` stores:**

- **Operational** — `name` and `profile` (profile name reference). Used for project identity and as the lookup key into the profile store.
- **Informational snapshot** — `workspace`, `workspace_id`, `environment`, `email`. Written at `init` time from the resolved profile for developer recognition when inspecting a project. These fields are never read for routing decisions; runtime always resolves from the profile's authoritative store.
- **Excluded** — `region` and `store_type`. Pure routing metadata with no recognition value; remains in the profile store only.

The informational snapshot is safe to persist because the entire `.wk/` directory is gitignored per ADR-005 Decision 8 — `wk.toml` is a local-only file and no PII or environment-specific data reaches version control.

### 9. Credential storage pattern (four-tier model)

Each credential store tier follows the same principle: the store is self-contained and authoritative for its own profiles. Profile metadata lives where the store lives.

| Tier | Backend | Metadata Location | Credential Location | CLI Writes? |
|------|---------|-------------------|---------------------|-------------|
| 1 | Secrets manager (Vault, AWS SM) | Remote store | Remote store | Connects + reads; provisioning is external |
| 2 | OS keychain | `~/.wk/profiles.json` | OS keychain | Yes (both) |
| 3 | Project-level file (`profiles.env`) | `profiles.env` | `profiles.env` | No (read-only) |
| 4 | AES-encrypted file | Encrypted file | Encrypted file | Yes (must, since encrypted) |

Keychain (Tier 2) is the only tier where metadata and credential are split across two locations. OS keychains are pure secret stores without structured enumeration — `profiles.json` serves as the enumerable index.

Tier 1 (secrets manager) requires local connection configuration (e.g., Vault address, IAM role) to reach the remote store. This configuration is a CLI-level setting, not profile-specific. Once connected, the secrets manager holds both profile metadata and credentials.

Tier 4 (AES-encrypted file) follows the same co-location pattern as Tier 3 but encrypted. The CLI must write to it since the developer cannot hand-edit an encrypted file. Likely stored at user level (`~/.wk/credentials.enc`) since air-gapped environments typically use a single credential set across projects.

Tiers 1 and 4 are Phase 1 deliverables (per ADR-001 Decision 5). This ADR establishes the storage pattern they must follow.

### 10. Non-interactive mode behavior

CI/CD and scripted invocations prioritize determinism over keystroke economy. The CLI detects non-interactive mode via `!isatty` on stdin or an explicit `--no-input` flag. In this mode, the login-flag resolution model narrows:

- **`--environment` is required explicitly.** The CLI does not probe `/activity_logs` or any other side-channel oracle for environment introspection in non-interactive mode, even if such introspection is introduced later. Making CI success depend on remote audit-log state would introduce flakiness (log retention, permission changes, empty workspaces).
- **`--workspace` is still introspected.** `GET /users/me` is a reliable oracle with no failure modes beyond auth itself, so introspection remains the default for consistency with interactive mode.
- **Computed `--name` still applies silently when omitted.** Auto-naming is not remote-dependent.
- **Any missing required flag hard-fails** with a message pointing the user at interactive mode for auto-detection.

This introduces an intentional asymmetry: `wk auth login --token X` behaves differently in a terminal versus a pipe. Documented explicitly in command help and CI setup docs so a developer copying a working interactive command into a pipeline is not surprised.

---

## Alternatives Considered

### Composite key (workspace + environment as the identifier)

Using `workspace/environment` as the profile lookup key instead of a separate `name`. This was rejected because:
- It forces a breaking change to every layer that passes a profile identifier (keyring, active_profile file, wk.toml, CredentialStore interface)
- It makes the `auth switch` command verbose (`wk auth switch acme-corp/dev` vs. `wk auth switch dev`)
- It requires migration of existing keychain entries and config files
- The developer-chosen `name` provides a convenient short alias that can be anything

### Optional workspace and environment

Making the new fields optional rather than required. This was rejected because optional fields would allow profiles to exist without targeting information, defeating the purpose of the change.

### Introspecting `--environment` from `/activity_logs` (April 17 revision)

Considered introspecting environment from `GET /activity_logs.workspace.environment` as part of the login ergonomic rework, to match the introspection treatment of `--workspace`. Deferred because `/activity_logs` has three distinct failure modes — empty logs on brand-new workspaces, 403 without audit-log permission, and network errors — each requiring distinct fallback behavior. Shipping this alongside workspace introspection would add meaningful complexity for a field developers already have a clear mental model for. The decision is to revisit once either (a) field data shows the failure modes are rare in practice, or (b) Platform adds a proper `/session` endpoint that returns `{workspace, environment}` directly. Separately, even when introspection ships, non-interactive mode will still require `--environment` explicitly (Sub-decision 10) to avoid making CI success depend on remote audit-log state.

---

## Consequences

### What becomes easier

- **Multi-environment workflows**: Developers can see at a glance which workspace and environment each profile targets, rather than relying on naming conventions.
- **Minimum-flag login**: In a TTY, `wk auth login --token X` is sufficient — workspace introspects from `/users/me`, name computes from `<region>-<workspace-slug>-<environment>`, region defaults to `us`. Only `--environment` is prompted. In non-interactive mode, the minimum is `--token` + `--environment`.
- **Project recognition**: `cat wk.toml` reveals which workspace, environment, and account a project targets without cross-referencing profile aliases or opening `~/.wk/profiles.json`.
- **CI/CD setup**: Pipelines produce a `profiles.env` file from their secrets manager — no `auth login` step, no keychain dependency, same profile schema as local dev.
- **Safety**: `auth list` output shows the actual target (workspace, environment, region), reducing the risk of operating against the wrong environment. `init` validates profiles exist and enforces active-profile consistency.
- **Consistency across store types**: The profile schema is universal. Whether a developer uses keychain locally or a file store in CI, the same fields are present and the same lookup-by-name behavior applies.
- **Deterministic credential routing**: StoreType-driven routing eliminates the guesswork of the ChainStore pattern. The developer (or their profile configuration) declares which backend to use.
- **Uniform flag resolution**: Every required profile field follows flag → default → prompt. The old mental split between "optional" and "required" flags collapses into "required with varying default source."

### What becomes harder

- **Login flow adds an API call**: `wk auth login` now calls `GET /users/me` before writing the profile, so token validation happens at login time rather than first-use. A failed introspection blocks profile creation — intentional, since a token that can't authenticate is not worth persisting.
- **`wk init` writes more fields**: `init` must resolve workspace, environment, and email from the active profile and write them to `wk.toml`. Minimal code addition but a new responsibility.
- **Two representations of workspace/environment**: The profile store remains authoritative; `wk.toml` holds a local snapshot. Drift is possible (profile retargeted, workspace renamed server-side). Drift is display-only — routing always uses the profile store — and handling is deferred pending field signal.
- **Interactive vs. non-interactive asymmetry**: `wk auth login --token X` behaves differently in a TTY vs. a pipe. Documented explicitly, but a developer copying a working interactive command into a CI script needs to know to add `--environment`.
- **File store setup**: Developers using the file store must manually create and manage `<project>/.wk/profiles.env`. The CLI does not scaffold it. Mitigated by `docs/profiles.env.example` and the simple key=value format.
- **`init` is stricter**: Profile must exist, active profile must match. Developers who previously used arbitrary profile names in `wk.toml` must now create the profile first. This is an intentional safety improvement.

### What we'll need to revisit

- **Environment introspection**: `GET /activity_logs.workspace.environment` is reachable today but has edge cases (empty logs, missing permissions). Deferred to a future revision pending field validation or a Platform-provided `/session` endpoint.
- **Interactive profile picker for `init`**: If developer demand warrants it, `init` could present a list of available profiles to select from rather than requiring the name as a flag. Deferred to avoid importing a picker dependency for a single command.
- **Tier 1 and 4 implementation**: The storage pattern is defined here; implementation details (Vault connection config, encrypted file key management) will be specified in dedicated ADRs when those tiers are built.

---

## Action Items

### Profile model
1. [x] Update `Profile` struct in `internal/auth/types.go` with `Workspace` and `Environment` fields
2. [x] Add workspace+environment+region uniqueness validation to `ProfileManager.SaveProfile()`
3. [x] Rename `Config.Workspace` to `Config.Profile` (struct + TOML tag)
4. [x] Rename `clone --workspace` to `clone --path-prefix`

### Auth commands
5. [x] Add `--workspace`, `--environment`, and `--force` flags to `wk auth login` (`--store-type` deferred to item 15)
6. [x] Add interactive prompt flow to `auth login` (prompt in struct field order when flags omitted in TTY mode)
7. [x] Add overwrite detection and confirmation prompt to `auth login`
8. [x] Implement `wk auth delete <name>` command
9. [x] Update `auth list` — add WORKSPACE, ENVIRONMENT columns (file store reading deferred to item 14)
10. [x] Update `auth status` — show workspace/environment, warn when fields are empty (file store resolution deferred to item 14)
11. [x] Update `auth switch` to validate against both `profiles.json` and `profiles.env` (when in a project directory)

### Credential routing
12. [x] Replace `ChainStore` usage in `resolveAPIClient` with StoreType-driven routing (Sub-decision 6 resolution flow)
13. [x] Remove `EnvStore` (process env var credential reader)
14. [x] Implement `FileStore` — reads `profiles.env`, parses multi-profile key=value format, looks up by `NAME=`
15. [x] Add `--store-type` as a global flag for explicit backend override

### Init validation (cohesive with ADR-005)
16. [x] Add profile existence validation to `wk init`
17. [x] Add active profile mismatch check to `wk init` (hard fail, name the mismatch)
18. [x] Add `--store-type` flag to `wk init` for file-store profile validation
19. [x] Warn and defer when `--store-type file` is specified but no `profiles.env` exists

### Documentation and cleanup
20. [x] Update all help text, error messages, and prompts for terminology consistency
21. [x] Remove `WK_TOKEN`, `WK_REGION` env var references from code and documentation
22. [x] Create `profiles.env.example` documenting the `profiles.env` format
23. [x] Update all tests (for completed items; remaining tests ship with their respective items)

### Login introspection and flag resolution (April 17, 2026 revision)
24. [x] Clarify `WorkspaceUser` struct semantics in `internal/api/types.go` — the `/users/me` response represents the workspace, not a human user; rename or split the type to stop the semantic mislabel
25. [x] Add `WorkspaceID int` and `Email string` fields to the `Profile` struct in `internal/auth/types.go` (populated at login from `/users/me`; read at init time to populate the `wk.toml` snapshot)
26. [x] Call `GET /users/me` in `auth login` as step 2 of the interactive flow (before environment/name prompts); use the response to populate `workspace`, `workspace_id`, and `email` on the profile. Failure aborts login.
27. [x] Change `--workspace` on `auth login` from prompted/required to optional override; error if provided and the value mismatches the introspected workspace
28. [x] Implement `--name` auto-compute (slugify workspace name + `-<environment>` + conditional `-<region>` suffix when non-default); show the computed default in interactive prompts as `[default]:`
29. [x] Enforce the non-interactive mode contract on `auth login`: detect `!isatty`/`--no-input`, hard-fail when required flags (particularly `--environment`) are missing, keep `/users/me` introspection enabled

### `wk.toml` schema extension (April 17, 2026 revision)
30. [x] Add `Workspace string`, `WorkspaceID int`, `Environment string`, and `Email string` fields to the `Config` struct in `internal/config/config.go` with matching TOML tags
31. [x] Update `wk init` to resolve and write the four new `wk.toml` fields at project creation (read from the resolved profile's record)
32. [x] Update `auth login` command help text and project docs to describe the reduced flag surface, `/users/me` introspection, and the new `wk.toml` snapshot fields
33. [x] Document the interactive vs. non-interactive mode asymmetry in CI setup docs

### Post-beta-test alignment (surfaced during manual testing)
36. [x] Align `wk init` non-interactive contract with `wk auth login`: detect non-interactive mode via `!isatty(stdin) && !--no-input && !--json` (previously `--json`-only), fail fast on missing required flags (`--name`, `--profile`) before any prompt label prints, and add a `--no-input` flag for explicit non-interactive mode.
37. [x] Harden interactive detection to require BOTH stdin and stdout to be TTYs. Captured-output contexts (`wk auth login --token X > out.log`) now correctly route through the non-interactive fail-fast path instead of printing orphaned prompt labels.
38. [x] Consolidate non-interactive missing-flag errors: one error listing all missing required flags instead of one per flag.
39. [x] Auto-name format change: `<region>-<workspace-slug>-<environment>` with region always leading. Previously region was omitted for `us` and suffixed for others, which hid the most discriminating field in the common case.
40. [x] Region expansion: add `il` (`app.il.workato.com`) and `cn` (`app.workatoapp.cn` — note the distinct `.workatoapp.cn` domain, not the standard `app.<region>.workato.com` pattern; Workato's allowlist docs at https://docs.workato.com/en/security/ip-allowlists.html confirm this). Drift guard `TestBaseURL_AllRegions` ensures `config.RegionURLs` stays aligned with `auth.ValidRegions()` going forward.
41. [x] Consolidate duplicate BaseURL mapping: `internal/auth/file.go` now routes through `config.BaseURL` instead of maintaining its own region → URL switch. The "avoid circular import" concern that justified the duplication is stale (config does not import auth).
42. [x] Fix `profiles.env` location bug: `NewFileStore` was anchored at `<projectRoot>/profiles.env`, but the original design ("alongside `wk.toml`") meant the file should have moved into `.wk/` when ADR-005 relocated `wk.toml`. The path is now `<projectRoot>/.wk/profiles.env`, which also puts it under the `.wk/.gitignore` self-ignore for free. The example file moved from the repo root to `docs/profiles.env.example` so its presence can't mislead anyone into dropping a real `profiles.env` at the wrong location.

### Deferred to future revision
34. [ ] File a Platform request for a `/session` or `/whoami` endpoint returning `{workspace, environment}` — unblocks environment introspection without the `/activity_logs` workaround
35. [ ] Open a tracking issue for deferred work: environment introspection via `/activity_logs` (with documented fallback), drift detection between `wk.toml` snapshot fields and the currently-resolved profile
43. [ ] VPW (Virtual Private Workato) customer support — Workato's allowlist page notes VPW customers have private docs with their own hostname conventions. No public pattern we can encode yet; pending an internal contact or a published VPW configuration spec.
