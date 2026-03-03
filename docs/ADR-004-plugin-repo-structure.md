# ADR-004: Plugin Repo Structure — Separate Repos, Not a Monorepo

**Status:** Accepted
**Date:** March 3, 2026
**Author:** Zayne Turner
**Deciders:** DevRel Engineering
**References:** ADR-001 (Decision 6: JSON-RPC Plugins), ADR-002 (Decision 8: Pre-Push Hooks), wk-lint-beta ADR-0001 (Tiered Lint Architecture)

---

## Context

ADR-001 Decision 6 established JSON-RPC over stdio as the plugin protocol. ADR-002 Decision 8 established fail-open pre-push hooks. The next question was: where does a plugin's source code live?

The initial lint ADR (wk-lint-beta `docs/adr/0001-tiered-lint-architecture.md`) proposed that the linter core library would live at `wk-cli-beta/pkg/lint/` and the plugin binary at `wk-cli-beta/plugins/recipe-lint/` — a monorepo layout where the CLI and its first plugin shared a Go module. During implementation, we moved the plugin to its own repository (`wk-lint-beta`) with its own `go.mod`. This ADR documents that decision and why the separation matters for the plugin ecosystem.

---

## Decision

Plugins are separate repositories with independent Go modules, independent build/release cycles, and no import dependency on the CLI. The CLI discovers installed plugins at `~/.wk/plugins/` by scanning for `plugin.toml` manifests. The only contract between CLI and plugin is the JSON-RPC protocol over stdio and the `plugin.toml` manifest format.

---

## Key Design Decisions

### Decision 1: Separate Repos Over Monorepo

**Decision:** Each plugin lives in its own repository with its own `go.mod`. The first plugin, `recipe-lint`, lives at `github.com/workato-devs/wk-lint-beta`, not inside `wk-cli-beta`.

**What the original ADR proposed:**

```
wk-cli-beta/
├── pkg/lint/          ← linter core library
├── plugins/
│   └── recipe-lint/
│       ├── main.go    ← JSON-RPC server, imports pkg/lint
│       └── plugin.toml
```

**What we actually built:**

```
wk-cli-beta/                          wk-lint-beta/
├── internal/plugin/                  ├── cmd/recipe-lint/
│   ├── host.go      (PluginHost)    │   ├── main.go      (JSON-RPC server)
│   ├── rpc.go       (RPCClient)     │   └── main_test.go
│   ├── registry.go  (Registry)      ├── pkg/lint/
│   ├── manifest.go  (Manifest)      │   ├── lint.go       (LintRecipe orchestrator)
│   └── hooks.go     (RunPrePushHook)│   ├── diagnostic.go (LintDiagnostic types)
├── internal/commands/                │   ├── tier0_schema.go
│   ├── plugin.go    (wk plugins)    │   ├── tier1_steps.go
│   └── sync.go      (pre-push hook) │   ├── config.go     (.wklintrc.json)
                                      │   ├── rules.go      (ConnectorRules loader)
                                      │   └── testdata/
                                      ├── pkg/recipe/
                                      │   ├── parse.go      (recipe JSON parser)
                                      │   └── types.go      (Recipe, Code, FlatStep)
                                      ├── plugin.toml
                                      ├── go.mod           (independent module)
                                      └── Makefile
```

**Why the divergence:**

The monorepo layout creates a coupling problem that the JSON-RPC protocol was specifically designed to avoid. If the linter imports `wk-cli-beta/pkg/lint`, then the plugin binary is compiled against a specific version of the CLI's Go module. Updating a lint rule requires updating the CLI module, which may pull in unrelated dependency changes. The plugin's release is gated on the CLI module's state.

The whole point of JSON-RPC is that the plugin is a separate process — the CLI doesn't need to know what language the plugin is written in, what dependencies it has, or how it's built. A monorepo `import` path undermines that boundary by creating a compile-time dependency where only a runtime (protocol) dependency should exist.

With separate repos, the linter team can ship a new rule by pushing to `wk-lint-beta` and cutting a release. No CLI PR needed. No CLI dependency update needed. The user runs `wk plugins install <path>` and gets the new binary.

**Alternative considered:** Git submodules to include `wk-lint-beta` inside `wk-cli-beta`. Rejected because submodules add Git complexity without solving the coupling problem — the `import` path still creates a compile-time dependency.

### Decision 2: `plugin.toml` as the Plugin Manifest

**Decision:** Every plugin declares its identity, entrypoint, commands, and hooks in a `plugin.toml` file at the plugin root.

**Schema (as implemented):**

```toml
name = "recipe-lint"
version = "0.1.0"
description = "Tiered validation for Workato recipe JSON"
entrypoint = "./recipe-lint"

[hooks]
pre-push = "lint.pre_push"

[[commands]]
name = "lint"
description = "Lint Workato recipe JSON files"
method = "lint.run"
```

**Why TOML:** Matches `wk.toml` for the CLI config (ADR-001 Decision 2). Developers working in the `wk` ecosystem encounter one config format.

**Manifest fields:**

| Field | Type | Purpose |
|-------|------|---------|
| `name` | string | Plugin identity, used as install directory name under `~/.wk/plugins/` |
| `version` | string | Semver version for display and future update checking |
| `description` | string | Human-readable description for `wk plugins list` |
| `entrypoint` | string | Relative path to the plugin binary. Resolved against the plugin's install directory. |
| `hooks.pre-push` | string | JSON-RPC method name for the pre-push hook. Empty string = no hook. |
| `hooks.post-pull` | string | Reserved, not dispatched yet. |
| `commands` | array | Commands the plugin registers on the CLI (name, description, method). |
| `commands.subcommands` | array | Nested subcommands under a plugin command (name, description, method, args). |

**Implementation:** `internal/plugin/manifest.go` — `Manifest` struct parsed via `go-toml/v2`. `LoadManifest()` reads and unmarshals the TOML file.

### Decision 3: Registry at `~/.wk/plugins/`

**Decision:** Installed plugins live in `~/.wk/plugins/<name>/`. The registry scans this directory for `plugin.toml` manifests.

**Why a global registry, not per-project:** Plugins are tools, not project dependencies. A linter plugin should be available for all projects on the machine, not re-installed per project. This matches the mental model of `git` plugins (global) rather than `npm` packages (per-project).

**Install flow:**

```bash
# Build the plugin
cd wk-lint-beta && make build

# Install into the registry (copies the entire directory)
wk plugins install .
# → copies to ~/.wk/plugins/recipe-lint/
```

**Implementation:** `internal/plugin/registry.go` — `Registry` struct with `Install()`, `List()`, `Remove()`, `GetPluginDir()`. `Install()` reads the `plugin.toml` at the source path to determine the plugin name, then copies the entire directory (binary + manifest + any data files) into `~/.wk/plugins/<name>/`.

### Decision 4: JSON-RPC over Stdio for Plugin Communication

**Decision:** The CLI spawns the plugin binary as a subprocess and communicates via JSON-RPC 2.0 over stdin/stdout. One newline-delimited JSON object per request/response.

**Why stdio, not sockets:** Stdio requires zero configuration — no port allocation, no socket files, no firewall rules. The plugin binary reads from stdin, writes to stdout. The CLI spawns the process, writes a request, reads a response. No service discovery needed.

**Wire protocol:**

```
CLI writes to plugin stdin:  {"jsonrpc":"2.0","method":"lint.run","params":{...},"id":1}\n
Plugin writes to stdout:     {"jsonrpc":"2.0","result":{...},"id":1}\n
```

**Lifecycle:**

1. CLI calls `NewRPCClient(entrypoint)` — spawns the binary, captures stdin/stdout pipes
2. CLI calls `client.Call(method, params)` — writes request, reads response (30s timeout)
3. CLI calls `client.Close()` — sends `shutdown` method, waits 5s, kills if needed

**Plugin-side implementation pattern** (as seen in `wk-lint-beta/cmd/recipe-lint/main.go`):

- `bufio.Scanner` on stdin, reads one line at a time
- Unmarshal as `RPCRequest`, switch on `Method`
- Marshal result as `RPCResponse`, write to stdout with newline
- On `shutdown` method, respond and `os.Exit(0)`

**Implementation:** `internal/plugin/rpc.go` — `RPCClient` struct with `Call()` and `Close()`. `internal/plugin/host.go` — `PluginHost` manages multiple loaded plugins, routes calls by plugin name.

### Decision 5: Plugin Commands Extend the CLI

**Decision:** Plugins can register top-level commands on the CLI. The `recipe-lint` plugin registers `wk lint` as a command that delegates to the `lint.run` JSON-RPC method.

**How it works:** The `[[commands]]` array in `plugin.toml` declares command names and their corresponding RPC methods. When `wk lint <args>` is invoked, the CLI:

1. Finds the `recipe-lint` plugin in the registry
2. Loads the manifest to find the `lint` command → `lint.run` method
3. Spawns the plugin binary via `PluginHost.Load()`
4. Calls `PluginHost.Execute("recipe-lint", "lint.run", params)`
5. Formats and prints the result

**Subcommand support:** Commands can have nested subcommands with their own methods and argument definitions. The `Subcommand` struct includes `Args` for positional argument metadata.

**Implementation:** `internal/plugin/manifest.go` — `Command` and `Subcommand` structs. `internal/commands/plugin.go` — `wk plugins install|list|remove` management commands.

### Decision 6: wk-lint-beta Internal Architecture

**Decision:** The linter repo follows a two-package layout: `pkg/lint/` for the core validation library and `pkg/recipe/` for recipe JSON parsing. The plugin binary at `cmd/recipe-lint/` imports both and wraps them in a JSON-RPC server.

**Why `pkg/` not `internal/`:** The lint library is exported. Future consumers (CI tools, editor extensions, other plugins) can import `github.com/workato-devs/wk-lint-beta/pkg/lint` directly as a Go library — they're not forced to go through JSON-RPC. The plugin binary is one consumer of the library, not the only one.

**Tiered validation architecture:**

| Tier | Scope | Implementation | Status |
|------|-------|----------------|--------|
| 0 | Schema validation (valid JSON, required keys, types) | `tier0_schema.go` | Implemented |
| 1 | Step-level rules (numbering, UUIDs, providers, control flow) | `tier1_steps.go` | Implemented |
| 1b | Datapill validation (formula mode, interpolation) | `tier1_datapills.go` | Planned |
| 2 | Inter-step structure (requires IGM graph) | `tier2_structure.go` | Planned |
| 3 | Cross-step data flow (requires IGM alias map) | `tier3_dataflow.go` | Planned |

Tier 0 errors halt evaluation — if the JSON isn't structurally valid, higher-tier rules that depend on parsed structures would produce misleading results.

**Connector-specific extensibility:** The `ConnectorRules` type loads `lint-rules.json` data files from a `--skills-path` directory. Adding rules for a new connector means adding a JSON file, not writing Go code. The Go rules engine applies generic checks (required fields, EIS validation, type checks) parameterized by connector-specific data.

**Configuration:** `.wklintrc.json` at the project root controls rule severity overrides and file ignore patterns. `LoadConfig()` returns nil for missing files (no error), so projects without config get default behavior.

---

## What This Means for Plugin Authors

A plugin author needs:

1. **A binary** that reads JSON-RPC requests from stdin and writes responses to stdout
2. **A `plugin.toml`** declaring the plugin's name, entrypoint, commands, and hooks
3. **Nothing from the CLI repo** — no Go imports, no shared types, no build dependency

The CLI's `internal/plugin/hooks.go` defines the hook protocol types (`HookParams`, `HookResult`, `Diagnostic`), but the plugin doesn't import these types — it reimplements them locally (as `wk-lint-beta/cmd/recipe-lint/main.go` does with `prePushParams`, `prePushResult`, `prePushDiagnostic`). The JSON wire format is the contract, not the Go types.

This means plugins can be written in any language. A Python linter, a Rust formatter, a Node.js code generator — all viable, all installable via `wk plugins install <path>`.

---

## What Stays from the Original Lint ADR

| Original Proposal | Status | Notes |
|-------------------|--------|-------|
| `pkg/lint/` core library | **Implemented** (in wk-lint-beta, not wk-cli-beta) | Same package structure, different repo |
| `plugin.toml` manifest | **Implemented** | Exact format as proposed |
| `pre-push` hook protocol | **Implemented** | JSON-RPC method `lint.pre_push` |
| `wk lint` plugin command | **Implemented** | Delegates to `lint.run` method |
| Tiered architecture (0-3) | **Partially implemented** | Tiers 0-1a built, 1b-3 planned |
| `.wklintrc.json` config | **Implemented** | Rule overrides + file ignore patterns |
| Connector-specific `lint-rules.json` | **Implemented** | Loaded from `--skills-path` |

## What Diverged from the Original Lint ADR

| Original Proposal | Actual | Reason |
|-------------------|--------|--------|
| `pkg/lint/` in CLI monorepo | `pkg/lint/` in `wk-lint-beta` repo | Compile-time coupling undermines JSON-RPC protocol boundary |
| Plugin binary at `wk-cli-beta/plugins/recipe-lint/` | Plugin binary at `wk-lint-beta/cmd/recipe-lint/` | Independent repo, independent releases |
| GoReleaser builds plugin alongside CLI | Plugin has its own `Makefile` + build | Independent build pipeline |
| `validate` subcommand alias | Not yet wired | Deferred to CLI `wk recipes` command integration |

---

## Consequences

### What Becomes Easier

- **Independent release cadence**: New lint rules ship without a CLI release. The linter team pushes to `wk-lint-beta`, builds, users update via `wk plugins install`.
- **Polyglot plugins**: The protocol is language-agnostic. Any team can build a plugin in their preferred language.
- **Plugin testing in isolation**: `wk-lint-beta` has its own test suite (`go test ./...`) that doesn't require the CLI to be present or buildable.
- **Clean dependency trees**: The linter's `go.mod` has zero dependencies beyond the Go standard library. No transitive CLI dependencies.

### What Becomes Harder

- **Type synchronization**: The CLI's `HookResult`/`Diagnostic` types and the plugin's local copies must stay in sync manually. A breaking protocol change requires coordinated updates. (Mitigated by the protocol being simple and stable.)
- **Local development workflow**: Building and testing the full push → hook → lint flow requires two repos checked out and the plugin installed into `~/.wk/plugins/`. (Mitigated by each repo being independently testable.)
- **Discovery of available plugins**: There is no plugin registry or marketplace yet. Users must know the plugin repo URL and build from source. (Future: `wk plugins install recipe-lint` fetches from a release URL.)

### What We'll Need to Revisit

- **Plugin registry/marketplace**: When more plugins exist, `wk plugins install <name>` should resolve to a GitHub release or CDN URL without requiring the user to clone and build.
- **Plugin versioning and updates**: `wk plugins update` to check for and install newer versions.
- **Protocol versioning**: If the hook protocol changes (new fields, new methods), a version negotiation handshake during plugin load would prevent incompatible combinations.

---

## Action Items

1. [x] Implement plugin host (PluginHost, RPCClient) in CLI
2. [x] Implement plugin registry (Install, List, Remove) in CLI
3. [x] Implement plugin manifest parsing (LoadManifest) in CLI
4. [x] Implement pre-push hook dispatch (RunPrePushHook) in CLI
5. [x] Implement `wk plugins install|list|remove` commands
6. [x] Build recipe-lint plugin as separate repo (wk-lint-beta)
7. [x] Implement Tier 0 + Tier 1a lint rules
8. [x] Implement `.wklintrc.json` configuration
9. [x] Implement connector-specific `lint-rules.json` loading
10. [ ] Build plugin registry/marketplace for remote install
11. [ ] Add `wk plugins update` command
12. [ ] Wire `wk recipes validate` alias to lint plugin
