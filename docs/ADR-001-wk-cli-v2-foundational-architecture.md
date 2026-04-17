# ADR-001: Workato CLI v2 (`wk`) — Foundational Architecture

**Status:** Proposed
**Date:** March 3, 2026
**Author:** Zayne Turner
**Deciders:** DevRel Engineering, Platform CLI Team, Developer Experience Lead
**Supersedes:** n/a (greenfield rewrite)

---

## Context

The current `workato-platform-cli` is a Python-based tool with systemic problems that limit developer adoption and block agentic AI use cases. Over 30% of recent merged PRs are dependency fixes. The install path requires Python 3.11+, virtual environments, and is subject to recurring breakage (urllib3, pydantic, package name confusion). The command grammar lacks project-level abstractions, and the tool was designed interactive-first in a world where AI agents are now first-class CLI consumers.

Workato's platform surface has expanded — MCP servers, API collections, deeper connector introspection — and the CLI hasn't kept pace. Workato developers need a CLI that installs in seconds, covers the full Developer API, feels familiar to developers who use `gh`/`sf`/`terraform`, and works equally well when called by Claude, Cursor, or a CI pipeline.

This ADR establishes the foundational architecture for `wk`, the ground-up rewrite. It is the single decision record that downstream ADRs (plugin protocol, MCP delegation, sync engine) will reference as their architectural baseline.

---

## Decision

Build `wk` as a statically-compiled Go binary using Cobra for command routing, Viper for TOML-based configuration, and Bubble Tea for interactive terminal UIs. Distribute as a single binary with no runtime dependencies. Use TOML for all configuration files and JSON-RPC over stdio for plugin communication. Implement a tiered credential model with four storage backends. Auto-generate the API client from Workato's OpenAPI spec.

The rest of this document breaks this down into its constituent decisions with options considered and trade-off analysis for each.

---

## Decision 1: Implementation Language — Go

### Options Considered

#### Option A: Go

| Dimension | Assessment |
|-----------|------------|
| Complexity | Low — Cobra/Viper ecosystem is mature and well-documented |
| Binary distribution | Single static binary, cross-compiles from one CI build |
| Concurrency model | Goroutines — trivial parallel API calls, no async/await complexity |
| Team familiarity | Moderate — standard for CLI tooling, large hiring pool |
| Ecosystem precedent | `gh`, `terraform`, `docker`, `kubectl` |

**Pros:** Zero-dependency install eliminates the #1 adoption blocker. Cross-compilation is built into the toolchain. Goroutines make parallel API calls (list recipes across folders, batch tag operations) straightforward without the cognitive overhead of Python's asyncio. The CLI ecosystem is standardized around Go — Cobra alone powers `kubectl`, `hugo`, and `gh`.

**Cons:** Slightly more verbose than Python for string manipulation and quick scripting. Error handling is explicit (no exceptions), which adds boilerplate. Less flexible for rapid prototyping compared to a dynamic language.

#### Option B: Rust

| Dimension | Assessment |
|-----------|------------|
| Complexity | High — steep learning curve, ownership model adds friction |
| Binary distribution | Single binary, cross-compilation requires more toolchain setup |
| Concurrency model | Excellent (tokio), but overkill for API-bound workloads |
| Team familiarity | Low — smaller contributor pool, slower onboarding |
| Ecosystem precedent | `ripgrep`, `bat`, `fd` (utilities, not platform CLIs) |

**Pros:** Memory safety guarantees. Excellent performance characteristics.

**Cons:** The CLI is network I/O bound, not CPU-bound — Rust's performance advantages don't materialize. The learning curve would slow contributor velocity without meaningful user-facing gain. Cross-compilation toolchain is more complex than Go's built-in support.

#### Option C: TypeScript (Node.js)

| Dimension | Assessment |
|-----------|------------|
| Complexity | Low — fast to write, large package ecosystem |
| Binary distribution | Requires Node.js runtime or bundler (pkg/nexe) |
| Concurrency model | Event loop + async/await — adequate but single-threaded |
| Team familiarity | High across the org |
| Ecosystem precedent | `sf` (Salesforce CLI), `vercel` |

**Pros:** Fastest path to a working prototype. Largest ecosystem of npm packages.

**Cons:** Requires a Node.js runtime, which reintroduces install friction — the exact problem we're solving. Bundling into a standalone binary (via `pkg` or `nexe`) produces 50–80 MB binaries with known compatibility issues. Salesforce's `sf` CLI uses this approach and their install story remains worse than Go single-binary.

#### Option D: Python (improved)

| Dimension | Assessment |
|-----------|------------|
| Complexity | Low — existing codebase could be refactored |
| Binary distribution | PyInstaller/Nuitka — fragile, large binaries, platform-specific |
| Concurrency model | asyncio — adds real complexity, already a pain point in v1 |
| Team familiarity | High — it's what we have today |
| Ecosystem precedent | `aws-cli` (notable for install complaints) |

**Pros:** No language switch. Existing code could theoretically be reused.

**Cons:** The dependency management problems are inherent to the Python packaging ecosystem, not fixable by better code. PyInstaller binaries are 100+ MB and break across OS versions. This option doesn't solve the root cause.

### Decision

**Go.** It eliminates the install friction problem entirely, provides a mature CLI framework ecosystem, and aligns with the tools our users already have muscle memory for.

---

## Decision 2: Configuration Format — TOML

### Options Considered

#### Option A: TOML

| Dimension | Assessment |
|-----------|------------|
| Ambiguity | None — single canonical representation per value |
| Agent compatibility | Excellent — LLMs generate valid TOML more reliably than YAML |
| Typing | Explicit — `"1.0"` is a string, `1.0` is a float, always |
| Ecosystem | Go library `pelletier/go-toml/v2` is mature and TOML v1.0 compliant |

**Pros:** No ambiguity (YAML has 5+ ways to represent a string, implicit boolean coercion). Explicit typing prevents an entire class of "works on my machine" config bugs. Creates a clear visual boundary: `wk.toml` is config, `*.recipe.json` is data. Agents generate correct TOML more reliably because there's only one valid way to express each value.

**Cons:** Less familiar to some developers than YAML. Deeply nested structures are more verbose than YAML.

#### Option B: YAML

| Dimension | Assessment |
|-----------|------------|
| Ambiguity | High — `yes`/`no`/`on`/`off` coerce to booleans, whitespace-sensitive |
| Agent compatibility | Poor — LLMs frequently produce invalid YAML (indentation errors, implicit type coercion) |
| Typing | Implicit — `version: 1.0` is a float, not a string |
| Ecosystem | Ubiquitous |

**Pros:** Universal familiarity. Every developer has edited a YAML file.

**Cons:** Ambiguity is a real operational risk when both humans and AI agents write config. The Norway problem (`NO` → `false`) is not hypothetical — it's a class of bug we'd be signing up for.

#### Option C: JSON

| Dimension | Assessment |
|-----------|------------|
| Ambiguity | None — strict grammar |
| Agent compatibility | Excellent — LLMs generate valid JSON reliably |
| Typing | Moderate — no date type, no comments |
| Ecosystem | Universal |

**Pros:** Zero ambiguity. Agents handle it well. Universal tooling.

**Cons:** No support for inline comments, which limits developer annotation of config files. Verbose for nested config. Most critically, recipe exports are also JSON — using JSON for config creates real ambiguity about file purpose. An agent or developer encountering a `.json` file in a project directory has no immediate way to know whether it's configuration or recipe data. This is exactly the kind of confusion that leads to accidental overwrites, misconfigured sync mappings, and wasted debugging time.

### Decision

**TOML.** The agent-friendliness argument is decisive: when both humans and LLMs write config, the format with zero ambiguity and explicit typing wins. The config/data visual boundary (`wk.toml` vs. `*.recipe.json`) is an additional benefit.

---

## Decision 3: Command Grammar — `wk <resource> <verb> [flags]`

### Rationale

The grammar follows the `<tool> <resource> <verb>` pattern used by `sf`, `gh`, `az`, and `kubectl`. Resources are plural nouns (`recipes`, `connections`, `folders`). Verbs are consistent across resources (`list`, `get`, `create`, `update`, `delete`).

This pattern was chosen over alternatives:

- **`wk <verb> <resource>`** (e.g., `wk list recipes`): Less common in the CLI ecosystem our users already know. `kubectl get pods` works, but `gh repo list` (resource-first) is the more common pattern in modern CLIs.
- **Flat command namespace** (e.g., `wk list-recipes`): Doesn't scale. With 60+ commands across 10+ resources, a flat namespace becomes unnavigable.

Every command supports `--json`, `--toml`, `--quiet`, `--verbose`, `--profile` (override active auth profile by name), `--no-color`, and `--timeout` as cross-cutting flags. Non-interactive mode is the default when stdout is not a teletypewriter (TTY) — i.e., when the output is being piped to another program or captured by a script rather than displayed in a human's terminal, ensuring agent and CI compatibility without requiring explicit flags.

---

## Decision 4: Project Model — Profile / Workspace / Environment / Project / Folder

### Rationale

The current CLI conflates "project" (a local development concern) with "folder" (a server-side organizational concern), and originally conflated "workspace" with "profile." This ADR establishes five distinct concepts:

- **Workspace**: A Workato account (e.g. `acme-corp`). An organizational boundary — all assets, connections, and recipes live within a workspace.
- **Environment**: A target within a workspace (e.g. `dev`, `staging`, `prod`). Represents a distinct deployment stage or isolation boundary.
- **Profile**: A named authentication configuration that targets a specific workspace + environment + region combination. Created via `wk auth login`. One profile is active at a time. The profile `name` is a developer-chosen identifier (e.g. `"dev"`, `"acme-prod"`) and serves as the primary lookup key throughout the CLI.
- **Project**: A local directory containing `wk.toml`. This is what a developer clones, branches, and PRs. It maps to one or more server-side folders via `[[sync]]` entries.
- **Folder**: A Workato server-side organizational container. Referenced by `absolute_path` in sync config.

The `wk.toml` manifest pins a project to a specific profile via the `profile` field (e.g. `profile = "dev"`). The CLI errors if the active profile doesn't match, preventing accidental cross-environment operations. This is the single most important safety invariant in the system.

See **ADR-006** for the full decision record on the profile identity model, including the addition of `workspace` and `environment` fields.

Local metadata (`.wk-meta.json` sidecar files) tracks each asset's server-side `absolute_path`, `zip_name`, and `folder` values. This ensures `wk push` can reconstruct the correct zip structure for import — the path problem documented in Appendix A of the PRD.

---

## Decision 5: Credential Storage — Tiered Model

### Options Considered

#### Option A: OS Keychain Only

**Pros:** Simple implementation. Encrypted at rest.

**Cons:** Often prohibited in regulated enterprise environments. No audit trail, no rotation, no cross-machine portability. Not usable in CI/CD.

#### Option B: Environment Variables Only

**Pros:** Universal CI/CD compatibility. Zero implementation complexity.

**Cons:** Plaintext in process environment. Leaked via `/proc`, child processes, and logs. The GhostAction attack (September 2025) exfiltrated 3,325 secrets from GitHub Actions env vars. Increasingly seen as a liability.

#### Option C: Tiered Model (Four Backends)

**Pros:** Meets every deployment context — enterprise (Vault/AWS SM), developer (keychain), CI/CD (env vars), air-gapped (encrypted file). The credential backend is a Go interface (`CredentialStore`), so new backends are additive.

**Cons:** More implementation surface. Four code paths to test and maintain.

### Decision

**Tiered model.** The credential backend is pluggable via a `CredentialStore` interface:

| Tier | Backend | Default For |
|------|---------|------------|
| 1 | Secrets manager (Vault, AWS SM, Doppler) | Enterprise / regulated |
| 2 | OS keychain | Interactive developer use |
| 3 | Project-level file (`profiles.env`) | CI/CD pipelines |
| 4 | AES-encrypted file | Air-gapped / constrained (explicit opt-in) |

The POC implements Tier 2 and Tier 3. Tier 1 and Tier 4 are Phase 1 deliverables.

> **Revision (April 17, 2026):** Tier 3 was originally a process env-var
> reader (`WK_TOKEN`/`WK_REGION`). [ADR-006](./ADR-006-profile-identity-model.md)
> replaces it with a project-level file (`profiles.env`) so the profile
> schema is uniform across all tiers and CI pipelines aren't coupled to a
> bespoke env-var contract.

Key libraries: `go-keyring` (keychain), `hashicorp/vault/api` (Vault), `aws-sdk-go-v2` (AWS SM), `crypto/aes` (encrypted file).

---

## Decision 6: Plugin System — JSON-RPC over Stdio

### Options Considered

#### Option A: Exec-based (run script, capture stdout)

| Dimension | Assessment |
|-----------|------------|
| Simplicity | High — shell out, read stdout |
| Bidirectional communication | None |
| Streaming | Not supported |
| Plugin-to-CLI callbacks | Not possible |

**Pros:** Dead simple to implement. Any script is a plugin.

**Cons:** No way for a plugin to call back into the CLI for context (e.g., query recipes, check workspace state). No streaming output. No typed schemas. Fundamentally one-directional.

#### Option B: JSON-RPC over Stdio

| Dimension | Assessment |
|-----------|------------|
| Simplicity | Moderate — requires protocol implementation |
| Bidirectional communication | Full — CLI↔Plugin messages in both directions |
| Streaming | Supported via notifications |
| Plugin-to-CLI callbacks | Native — plugins call `wk.recipes.list`, etc. |

**Pros:** Bidirectional: plugins can call back into the CLI for context. Typed request/response schemas. Same protocol pattern as MCP and LSP — plugin authors building MCP-aware tools are already thinking in this model. Any language that reads/writes JSON to stdin/stdout can be a plugin.

**Cons:** More complex implementation than exec. Requires a protocol handler on both sides.

#### Option C: gRPC

| Dimension | Assessment |
|-----------|------------|
| Simplicity | Low — requires protobuf definitions, code generation |
| Bidirectional communication | Full |
| Streaming | Excellent |
| Plugin-to-CLI callbacks | Native |

**Pros:** Strong typing via protobuf. Excellent streaming support. Battle-tested at scale.

**Cons:** Heavy for a CLI plugin system. Requires protobuf toolchain. Overkill when we need "any language that can do JSON over stdin/stdout."

### Decision

**JSON-RPC over stdio.** The bidirectional communication is essential (plugins need to call back into the CLI), and the protocol aligns with MCP/LSP patterns our users already understand. The authorship bar stays low — no code generation required.

Plugin manifest is `plugin.toml`. Plugins register commands, lifecycle hooks (pre-push, post-pull), and AI-consumable skills. Plugin discovery: `wk plugins install <path-or-url>`, with commands appearing in the main `wk` namespace.

---

## Decision 7: API Client — Auto-Generated from OpenAPI Spec

### Rationale

The Workato Developer API client will be auto-generated from the OpenAPI specification using `oapi-codegen` (or `ogen` if stronger type safety is needed). The current Python CLI already uses a generated client — this continues the pattern.

Auto-generation ensures the CLI stays in sync with API changes without manual maintenance. The generated client is wrapped in a thin service layer that adds retry logic, pagination handling, and `--json`/`--toml` output formatting.

### Alternatives Considered

- **Hand-written client**: More control, but creates a maintenance burden as the API surface grows. Every new endpoint requires manual work.
- **Generic HTTP client with schema validation**: Flexible but loses the type safety and IDE support that a generated client provides.

---

## Decision 8: Distribution — Multi-Channel Single Binary

### Approach

```
brew install workato-devs/tap/wk        # macOS (Homebrew custom tap)
curl -fsSL https://get.workato.dev | sh  # Linux / macOS (shell installer)
scoop install wk                          # Windows
go install github.com/workato-devs/wk@latest  # Go developers
```

Plus GitHub Releases with prebuilt binaries for every OS/arch combination (macOS ARM + Intel, Linux amd64 + arm64, Windows amd64), and a Docker image for CI environments.

Build tooling: **GoReleaser** for cross-platform builds, checksum generation, and Homebrew formula automation.

### Target Constraints

| Metric | Target |
|--------|--------|
| Install time (clean machine) | < 30 seconds |
| Binary size | < 30 MB |
| Supported platforms | macOS (ARM + Intel), Linux (amd64 + arm64), Windows (amd64) |

---

## Decision 9: CLI Name — `wk`

### Options Considered

- **`wk`**: Two characters, fast to type, aligns with the `gh`/`sf`/`az`/`gp` pattern. No existing Homebrew formula in core. Two low-adoption open-source tools use the binary name (a wiki manager on SourceHut, a web app tool on GitHub) — neither is in Homebrew. Using a custom tap (`workato-devs/tap/wk`) avoids any conflict.
- **`work`**: Generic word — `work workspace info` stutters. Higher shell alias collision risk. Doesn't uniquely connote "Workato."
- **`wrk`**: Conflicts with [wrk](https://formulae.brew.sh/formula/wrk) (HTTP benchmarking tool in Homebrew core). Hard no.
- **`workato`**: Too long for frequent terminal use. Typing `workato recipes list` dozens of times a day is a tax on adoption.

### Decision

**`wk`**. Two characters, unambiguous, follows the modern CLI naming convention. Product name "Workato CLI" for marketing; binary name `wk` for the terminal.

---

## Decision 10: Mixed-State Push Behavior — Preserve by Default

### Rationale

When pushing recipes to a Workato workspace, the CLI must decide what to do about recipe running/stopped state. Three options:

- **Restart all recipes after push**: Dangerous — an accidental `wk push` could activate recipes that were intentionally stopped.
- **Stop all recipes after push**: Equally dangerous in the other direction — recipes that should be running get stopped.
- **Preserve current state (default)**: The CLI records each recipe's running/stopped state before push and restores it afterward.

### Decision

**`--preserve-state` is the default behavior.** `wk push` preserves recipe running/stopped state. Explicit flags override: `--restart-recipes` restarts all, `--activate <id>` activates specific recipes. This follows the principle that the CLI should never change server-side state that the developer didn't explicitly ask to change.

---

## Decision 11: Recipe-Skills Plugin — Separate but Blessed

### Rationale

Recipe skills (Claude-compatible knowledge packages that teach an LLM how to build recipes for specific connectors) are a key differentiator. The question is whether they ship inside the CLI binary or as a plugin.

- **Bundled in binary**: Simpler install, but increases binary size, couples release cycles, and forces every user to carry skills they may not use.
- **Separate plugin, officially maintained**: Keeps the CLI lean. Skills can iterate independently. Plugin is prominently documented and installable in one command.

### Decision

**Separate-but-blessed.** The recipe-skills plugin lives in the monorepo (`plugins/recipe-skills/`), is officially maintained by the DevRel team, and is prominently documented in onboarding guides. It is not bundled in the binary. Install: `wk plugins install github://workato-devs/wk-plugin-recipe-skills@v1`.

---

## Decision 12: Migration from v1

### Rationale

This is a clean rewrite, not a port. However, migration must be painless for existing users:

1. **`wk migrate`** reads existing `~/.workato/` config and creates equivalent `wk` profiles.
2. Existing project folders (created by v1 `workato pull`) are auto-detected and linkable via `wk link`.
3. A published command mapping guide documents the v1 → v2 command translations (most are 1:1).

### What We're Deliberately Dropping

- Interactive-only commands (every command must work non-interactively)
- The `guide` command (replaced by `wk --help`, man pages, and online docs)
- Python-specific tooling (Makefile targets for mypy, ruff, etc.)

`wk migrate` is scoped to Phase 4, after the core CLI is stable. The migration path is designed so that v1 and v2 can coexist on the same machine during the transition.

---

## Decision 13: Repository Structure — Monorepo

### Rationale

Single repository containing the CLI, the blessed recipe-skills plugin, shared packages, and internal libraries:

```
wk/
├── cmd/wk/              # Main binary entrypoint
├── internal/            # Private packages (auth, sync engine, API client)
│   ├── auth/            # Tiered credential model
│   ├── client/          # Generated API client + service layer
│   ├── config/          # wk.toml parsing, profile management
│   ├── plugin/          # JSON-RPC host, plugin lifecycle
│   ├── sync/            # Pull/push/diff engine, metadata tracking
│   └── output/          # --json, --toml, --quiet formatting
├── pkg/                 # Public packages (reusable by plugins)
│   └── jsonrpc/         # JSON-RPC protocol types and helpers
├── plugins/
│   └── recipe-skills/   # Blessed plugin — separate binary, same repo
├── api/                 # OpenAPI spec + generated client
├── scripts/             # Build, lint, release automation
├── .goreleaser.yml
├── Makefile
└── go.mod
```

Monorepo to start. Split later if the plugin ecosystem grows beyond the core team. This avoids premature repository proliferation while keeping the build simple.

---

## Consequences

### What Becomes Easier

- **Installation**: Single binary, zero runtime dependencies. `brew install` or `curl | sh` — done.
- **Agent integration**: Every command supports `--json`, non-interactive mode, and machine-readable exit codes out of the box.
- **API coverage expansion**: Auto-generated client means new API endpoints are a spec update + regeneration, not hand-written code.
- **Enterprise deployment**: Tiered credential model means security teams can require Vault/AWS SM without the CLI needing code changes.
- **Plugin ecosystem**: JSON-RPC over stdio means plugins can be written in any language and communicate bidirectionally with the CLI.

### What Becomes Harder

- **Contributor onboarding**: Go is less familiar to some team members than Python. Mitigated by Go's small language surface and excellent tooling.
- **Rapid prototyping**: Go is more verbose than Python for quick experiments. Mitigated by the mature Cobra/Viper/Bubble Tea ecosystem.
- **Four auth backends**: More test surface than a single credential store. Mitigated by the `CredentialStore` interface abstracting the backends behind a single contract.
- **JSON-RPC complexity**: More protocol machinery than exec-based plugins. Mitigated by providing a `pkg/jsonrpc` helper package and a reference plugin implementation.

### What We'll Need to Revisit

- **MCP auto-delegation strategy** (Phase 4): How the CLI decides when to call the Workato API directly vs. delegate to an org's MCP server. Will need its own ADR once the MCP server landscape matures.
- **Multi-sync conflict resolution**: When multiple `[[sync]]` entries overlap or reference the same server-side folder, the conflict resolution strategy needs definition. Deferred to Phase 2.
- **Session token support**: When Workato's API supports short-lived OAuth2 tokens, the credential model should prefer those over long-lived API keys. This is a future Tier 0 that supersedes all other tiers.
- **Plugin security model**: The current design trusts plugins fully. A sandboxing or capability model may be needed as the plugin ecosystem grows beyond the core team.
- **Windows testing**: Go cross-compiles to Windows, but the keychain integration (`go-keyring` on Windows Credential Manager) and shell installer need dedicated testing.

---

## Forward References

The following topics are architecturally significant but warrant their own ADRs:

- **ADR-002: Sync Engine** — Pull/push/diff mechanics, `.wk-meta.json` sidecar design, server-side path identity tracking (`absolute_path` / `zip_name` / `folder`), cross-project reference detection, multi-sync conflict resolution. This is where the path problem (PRD Appendix A) gets its full architectural treatment.
- **ADR-003: MCP Delegation Strategy** — When the CLI calls the Workato API directly vs. delegates to an org's Dev API MCP server. Decision model, `auto_delegate` behavior, fallback semantics, and the `prefer_mcp_for` / `never_delegate` configuration. Design early (Phase 1), implement in Phase 4.
- **ADR-004: Plugin Security Model** — Capability-based sandboxing for third-party plugins. Not needed for POC (plugins are trusted, core-team authored), but required before opening the plugin ecosystem to the community.
- **ADR-006: Profile Identity Model** — Addition of `workspace` and `environment` as required metadata fields on the auth profile, renaming of the `wk.toml` field from `workspace` to `profile`, and terminology alignment across the CLI.

---

## Action Items

1. [ ] Circulate this ADR to DevRel Engineering and Platform CLI team for review
2. [ ] Finalize `CredentialStore` interface design (Tier 2 + Tier 3 for POC)
3. [ ] Set up Go module, Cobra skeleton, and GoReleaser config (POC Week 1)
4. [ ] Generate initial API client from Workato OpenAPI spec
5. [ ] Implement JSON-RPC stdio host and reference plugin (POC Week 2–3)
6. [ ] Publish Homebrew tap with POC binary
7. [ ] Draft ADR-002: Sync Engine (pull/push/diff, metadata tracking, path handling)
8. [ ] Draft ADR-003: MCP Delegation Strategy (Phase 4 scope, but design early)
