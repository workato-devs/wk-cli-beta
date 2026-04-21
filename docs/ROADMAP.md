# wk CLI — Build Roadmap

**Last updated:** 2026-03-09
**PRD reference:** `/Users/zayneturner/Cowork/CLI-redo/PRD-workato-cli-v2.md`

---

## Priority Order

| Priority | Scope | Status | Rationale |
|----------|-------|--------|-----------|
| **P0** | Workspace isolation check | **Done** | Safety — prevents cross-workspace push/pull accidents |
| **P1** | Phase 3: Platform coverage | **Done** (P1.3 CRUD blocked) | Tags, API Platform, Workspace, MCP test/tools complete |
| **P2** | Phase 2: Core CRUD expansion | **Done** | Recipe, connection, connector, folder commands complete |
| **P3** | Phase 1: Foundation gaps | Not started | Auth tiers 1 & 4, --toml output, --no-color |
| **P4** | Phase 4: Plugins & polish | Partial | pre-push hooks done; skills, bidirectional RPC, migrate not started |

---

## What's Done

### POC (Complete — 2026-02-18)

All items verified with 33-case E2E plan against live `app.trial.workato.com`.

- **Project lifecycle:** init, link, status, pull, push, diff, clone
- **Recipes:** list, get, start, stop, export, import
- **Connections:** list, get
- **Auth:** Tier 2 (keychain) + Tier 3 (env vars); login, list, switch, status
- **Sync engine:** .meta.json sidecars (under .wk/), multi-sync, SHA256 diff, preserve-state
- **Plugin system:** manifest parsing, install/list/remove, JSON-RPC dispatch, example plugin
- **Config:** wk.toml load/save, FindProjectRoot, [[sync]] entries, [mcp] struct
- **Output:** --json on all commands, text table formatting, --verbose, --quiet, --profile, --timeout
- **Build:** GoReleaser (6 platforms), Homebrew formula config, Makefile (7 targets)

### P0 — Workspace Isolation (Complete)

`checkWorkspaceMatch()` in `internal/commands/resolve.go`. Compares active profile against `wk.toml` workspace; `--profile` override bypasses the check.

### P1.1 — Tags (Complete)

All CRUD + apply/remove: `wk tags list/create/update/delete/apply/remove`.
Files: `internal/api/tags.go`, `internal/commands/tags.go`, `internal/api/tags_test.go`.

### P1.2 — API Platform (Complete — API-limited)

Collections: `wk api collections list/create`.
Endpoints: `wk api endpoints list/enable/disable`.
Files: `internal/api/api_collections.go`, `internal/api/api_endpoints.go`, `internal/commands/api.go`.

Note: Dev API does not expose single-get for collections or full CRUD for endpoints beyond enable/disable. Implementation matches what the API provides.

### P1.3 — MCP (Partial — CRUD blocked)

**Done:** `wk mcp test <url>`, `wk mcp tools <url>` — protocol-level probing, no Dev API dependency.
Files: `internal/mcp/client.go`, `internal/commands/mcp.go`.

**Blocked:** MCP server CRUD (list/get/create/update/delete) — no backing API endpoints exist. See [Architectural Findings](#architectural-findings) below.

### P1.4 — Workspace (Complete)

`wk workspace info/users/audit-log`.
Files: `internal/api/workspace.go`, `internal/commands/workspace.go`, `internal/api/workspace_test.go`.

### P2.1 — Recipe Gaps (Complete)

**Done:** `wk recipes jobs`, `wk recipes copy`, `wk recipes update-connection`, `wk recipes validate` (thin alias to `wk lint` plugin).

### P2.2 — Connection Gaps (Complete)

`wk connections create/update/delete/disconnect`.
Files: `internal/api/connections.go`, `internal/commands/connection.go`, `internal/api/connections_test.go`.

### P2.3 — Connectors (Complete — API-limited)

`wk connectors list [--search]`.
Dev API only exposes listing. Deep introspection (actions, triggers, field schemas) not available via Dev API.
Files: `internal/api/connectors.go`, `internal/commands/connector.go`, `internal/api/connectors_test.go`.

### P2.4 — Folders (Complete)

`wk folders list/create/delete`.
Files: `internal/api/folders.go`, `internal/commands/folder.go`, `internal/api/folders_test.go`.

### P4.1 — Plugin Hooks (Partial)

**Done:** `pre-push` hook support (`internal/plugin/hooks.go`).
**Not done:** `post-pull` hook.

### Quality — Struct & Type Coverage Tests (2026-03-09)

Added automated drift-detection tests:
- `internal/api/types_coverage_test.go` — reflection-based checks that structs capture all known API fields and table outputs include required columns.
- `internal/sync/pull_test.go` — known Workato export extension registry; fails if `inferAssetType()` returns "unknown" for a registered type.

See `CONTRIBUTING.md` for the workflow when adding fields or resources.

---

## Remaining Work

### P1.3 — MCP Server CRUD (Blocked)

**Blocker:** No MCP server CRUD endpoints in the Workato Dev API. Requires core platform team.

Needed endpoints:
```
GET    /api/mcp_servers
GET    /api/mcp_servers/{id}
POST   /api/mcp_servers
PUT    /api/mcp_servers/{id}
DELETE /api/mcp_servers/{id}
```

CLI commands ready to implement when API ships:
```
wk mcp list
wk mcp get <id-or-name>
wk mcp create --name <name> --collection <id>
wk mcp update <id> [--name NAME] [--collections IDS]
wk mcp delete <id>
```

### P3.1 — Auth Tier 1: Secrets Managers

Not started. Add `CredentialStore` implementations for:
- `VaultStore` — HashiCorp Vault (`hashicorp/vault/api`)
- `AWSSecretsManagerStore` — AWS SM (`aws-sdk-go-v2`)
- `DopplerStore` — Doppler API

Profile metadata already has `store_type` field in `auth.Profile`.

### P3.2 — Auth Tier 4: Encrypted File

Not started. `FileStore` — AES-GCM encrypted token file. Key derived from OS user + machine ID or passphrase.

### P3.3 — Auth Rotate

Not started.
```
wk auth rotate <profile>
```

### P3.4 — Output: --toml Flag

Not started. Add `"toml"` case to `internal/output/formatter.go` using `pelletier/go-toml/v2`.

### P3.5 — --no-color Wiring

Flag `flagNoColor` is parsed in `root.go` but **not wired** to formatters. Text formatter has no ANSI suppression logic. Quick fix (~15 lines).

### P4.1 — Plugin post-pull Hook

`pre-push` exists; `post-pull` not implemented.

### P4.2 — Plugin Skills Declarations

Not started. Manifest `[[skills]]` section for AI agent discovery metadata.

### P4.3 — Bidirectional JSON-RPC (Plugin → CLI Callbacks)

Not started. Plugin host needs reverse RPC handler registration.

### P4.4 — wk plugins update

Not started.
```
wk plugins update [name]
```

### P4.5 — wk migrate (from v1)

Not started.
```
wk migrate    → reads ~/.workato/ config, creates equivalent wk profiles
```

---

## Architectural Findings

### MCP Auto-Delegation — CUT (2026-02-19)

The Dev API MCP server is a thin REST passthrough (160 tools, 1:1 with endpoints). It adds no composite operations, returns Ruby hash notation instead of JSON, and uses the same dev API token. The PRD's delegation model (`prefer_mcp_for` / `auto_delegate`) is unnecessary. No delegation logic will be built.

### MCP Server Management — No API Exists (2026-02-19)

Two MCP server types in Workato:

1. **Dev API MCP** — `app.{region}.workato.com/mcp?dev_api_token=...`
   - 160 tools, 1:1 passthrough. Always available, not user-created.

2. **Collection MCP** — `{workspace_id}.apim.mcp.{region}.workato.com/{namespace}/{collection}?wkt_token=...`
   - Wraps API collection endpoints as MCP tools. Created via AI Hub UI only.
   - `wkt_token` is opaque, UI-generated, not retrievable via API.

13 REST paths probed for MCP management endpoints — all returned 404. MCP servers are a UI-layer abstraction. CI/CD pipelines need programmatic management — this requires a core platform API ask.

---

## Patterns & Conventions

See `CONTRIBUTING.md` for the workflow when adding resources, fields, or export extensions.

### Adding a New Resource Command (Checklist)

1. **Types** → `internal/api/types.go` — add struct with `json` tags
2. **Service interface** → `internal/api/client.go` — add `XxxService` interface
3. **Service impl** → `internal/api/<resource>.go` — implement using `HTTPClient.do()`
4. **Client accessor** → `internal/api/http_client.go` — add `Xxx() XxxService` method + lazy init
5. **Commands** → `internal/commands/<resource>.go` — Cobra commands following recipe.go pattern
6. **Register** → `internal/commands/root.go` → `registerAllCommands()` — add `root.AddCommand(newXxxCmd())`
7. **Tests** → `internal/api/<resource>_test.go` — httptest mock server, verify request/response
8. **Coverage** → `internal/api/types_coverage_test.go` — add struct to `TestStructFieldCoverage` and `TestStructFieldCoverage_TableColumns`

### Command Implementation Pattern

```go
func newXxxListCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "list",
        Short: "List xxx",
        RunE: func(cmd *cobra.Command, args []string) error {
            rctx, err := BuildRunContext(cmd)       // get formatters
            client, _, err := resolveAPIClient(cmd)  // get authenticated client
            items, err := client.Xxx().List(...)     // call service
            if flagJSON {
                return rctx.Formatter.Format(os.Stdout, items)  // JSON output
            }
            // text table output
            return rctx.Formatter.FormatList(os.Stdout, headers, rows)
        },
    }
    // flags
    return cmd
}
```

### API Response Patterns

Two response shapes in the Workato API:
- **Paginated with wrapper:** `{"items": [...], "count": N}` — recipes, projects
- **Bare array:** `[{...}, {...}]` — connections, folders, tags

Use `ListResult[T]` for the first pattern, direct `[]T` for the second.

### Testing Pattern

```go
func TestXxxList(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // verify method, path, headers
        // return mock JSON response
    }))
    defer srv.Close()
    client := api.NewHTTPClient(srv.URL, "test-token")
    result, err := client.Xxx().List(context.Background(), nil)
    // assert
}
```

---

## Live API Reference

The Workato Dev API MCP server at `app.trial.workato.com` exposes 160 tools across 30 resource domains.

| Domain | Tools | Phase | Status |
|--------|-------|-------|--------|
| `tags` + `tags_assignments` | 5 | P1.1 | **Done** |
| `api_collections` | 2 | P1.2 | **Done** |
| `api_endpoints` | 3 | P1.2 | **Done** |
| `api_access_profiles` | 6 | P1.2 | Ready |
| MCP server CRUD | 0 | P1.3 | **Blocked** — no API endpoints exist |
| MCP test/tools (protocol-level) | n/a | P1.3 | **Done** |
| `users` + `members` | 6 | P1.4 | **Done** |
| `activity_logs` | 1 | P1.4 | **Done** |
| `recipes` (remaining) | 12 | P2.1 | **Done** |
| `connections` (remaining) | 3 | P2.2 | **Done** |
| `integrations` | 2 | P2.3 | **Done** (list only — API-limited) |
| `folders` (remaining) | 2 | P2.4 | **Done** |

Dev API token and host are in `.env` (gitignored) for live testing.
