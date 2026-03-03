# ADR-003: MCP Strategy — No Auto-Delegation, Protocol-Level Tooling Only

**Status:** Accepted
**Date:** March 3, 2026
**Author:** Zayne Turner
**Deciders:** DevRel Engineering
**References:** ADR-001 (Foundational Architecture), PRD Section "MCP Integration Strategy"

---

## Context

The PRD proposed a sophisticated MCP delegation model: the CLI would detect when a Workato workspace has an MCP server available and automatically route certain commands through it instead of calling the REST API directly. The `wk.toml` config included `[mcp]` settings for `auto_delegate`, `prefer_mcp_for`, and `never_delegate`, allowing per-project control over which commands went through the MCP server versus the direct API.

The PRD also proposed full `wk mcp` CRUD commands (list, get, create, update, delete) for managing MCP servers programmatically.

During POC development, we probed the live Workato MCP infrastructure at `app.trial.workato.com` to validate these assumptions. What we found changed the decision significantly.

---

## Decision

**Cut MCP auto-delegation entirely.** Build `wk mcp test` and `wk mcp tools` as protocol-level diagnostic commands only. Defer MCP CRUD commands until Workato ships MCP management API endpoints.

---

## Findings from Live Probing (February 19, 2026)

### Two Distinct MCP Server Types Exist in Workato

**Dev API MCP** — always available, not user-created:
```
URL:    https://app.{region}.workato.com/mcp?dev_api_token={token}
Tools:  160 — full Dev API passthrough (auto-generated, 1:1 with REST endpoints)
Auth:   Dev API token (same as REST API)
Server: {"name":"my_server","version":"0.1.0"}
```

**Collection MCP** — user-created via the AI Hub UI:
```
URL:    https://{workspace_id}.apim.mcp.{region}.workato.com/{namespace}/{collection}?wkt_token={token}
Tools:  Only endpoints in the linked API collection (recipe-backed, custom schemas)
Auth:   wkt_token (opaque, UI-generated, not retrievable via any API)
Server: {"name":"API_platform","version":"1_0"}
```

### Why Auto-Delegation Adds No Value

The PRD's delegation model assumed the MCP server would provide richer capabilities than the raw REST API — composite operations, validation, enriched responses. The probing revealed the opposite:

The Dev API MCP is a **thin REST passthrough**. Each of its 160 tools maps 1:1 to a Dev API endpoint. It adds no composite operations, no validation, and no enriched responses. Worse, it returns Ruby hash notation (`=>`) instead of valid JSON, making its output strictly harder to parse than the REST API. It uses the same dev API token passed as a query parameter — no distinct auth model, no additional audit trail beyond what the platform already logs for all API token transactions.

Routing CLI commands through the MCP server would add a protocol layer (JSON-RPC over HTTP with SSE) on top of REST calls that the CLI already makes directly, with no functional benefit and a measurable latency cost. Every `wk recipes list` would become: CLI → JSON-RPC request → MCP server → REST API → MCP server → SSE response → CLI, instead of: CLI → REST API → CLI.

### Why MCP CRUD Commands Can't Be Built Today

We probed 13 REST endpoint paths for MCP server management:
```
/api/mcp_servers
/api/agentic/mcp_servers
/api/ai_hub/mcp_servers
/api/v2/mcp_servers
... (9 additional variations)
```

All returned 404. The MCP server abstraction is a UI-layer construct built on three existing primitives: API collections (define tools), API clients/keys (provide auth), and AI Hub config (generates the `wkt_token` and MCP URL). But the `wkt_token` — the credential that makes a Collection MCP URL work — is generated exclusively by the UI-based MCP server creation flow. It is not an API key, not retrievable via any Dev API endpoint, and not derivable from other credentials.

Without MCP management API endpoints from the core platform team, the CLI cannot programmatically create, list, update, or delete MCP servers.

---

## What We're Building

### `wk mcp test <url>` — Implemented

Performs an MCP protocol handshake against any MCP server URL. Reports server name, version, protocol version, and capabilities. Works with both Dev API MCP and Collection MCP URLs. Uses Streamable HTTP transport (POST with `Accept: text/event-stream`).

Implementation: `internal/mcp/client.go` → `Initialize()`, `internal/commands/mcp.go` → `newMCPTestCmd()`.

### `wk mcp tools <url>` — Implemented

Sends `initialize` followed by `tools/list` to any MCP server URL. Returns the full tool inventory with names, descriptions, and input schemas. Useful for developers and agents discovering what a Collection MCP server exposes.

Implementation: `internal/mcp/client.go` → `ListTools()`, `internal/commands/mcp.go` → `newMCPToolsCmd()`.

Both commands accept any MCP URL as a positional argument — they don't depend on the Dev API, workspace auth, or any `wk.toml` configuration. They are pure MCP protocol clients.

### `wk mcp url` — Deferred

The PRD proposed `wk mcp url <id-or-name>` to print the MCP URL for a server. This requires the MCP management API to look up servers by name. Deferred until that API exists.

---

## What We're Cutting

| PRD Feature | Status | Reason |
|-------------|--------|--------|
| `auto_delegate` routing | **Cut** | Dev API MCP is a 1:1 REST passthrough — delegation adds latency with no functional benefit |
| `prefer_mcp_for` / `never_delegate` config | **Cut** | No delegation means no routing rules needed |
| MCP client transport for delegation | **Cut** | The `internal/mcp/client.go` exists for `test`/`tools` commands, not for routing CLI commands through MCP |
| `wk mcp list` | **Blocked** | No `/api/mcp_servers` endpoint exists |
| `wk mcp get <id>` | **Blocked** | No single-get endpoint exists |
| `wk mcp create` | **Blocked** | No create endpoint; `wkt_token` generation is UI-only |
| `wk mcp update <id>` | **Blocked** | No update endpoint exists |
| `wk mcp delete <id>` | **Blocked** | No delete endpoint exists |

### What Stays in `wk.toml`

The `[mcp]` config section remains in the `wk.toml` schema. It does no harm, and if Workato ships MCP management APIs or enriches the MCP server beyond passthrough in the future, the config surface is already there. The CLI simply ignores it today.

```toml
[mcp]
auto_delegate = true              # parsed but not acted on
server_url = ""                   # parsed but not acted on
```

---

## Options Considered

### Option A: Build Auto-Delegation as Designed in the PRD

**Pros:** Matches the PRD. Future-proofs for a world where the MCP server adds value beyond passthrough.

**Cons:** Adds real complexity (MCP client transport in the hot path, routing logic, fallback handling, config surface) for zero user benefit today. The Dev API MCP returns worse output (Ruby hash notation) than the REST API. Building this would mean maintaining a slower, less reliable code path that produces harder-to-parse output.

### Option B: Cut Delegation, Build CRUD Commands Only

**Pros:** Gives developers `wk mcp list/create/delete` for managing servers from the CLI.

**Cons:** The backing API doesn't exist. We'd be shipping commands that fail with 404s, which is worse than not shipping them at all.

### Option C: Cut Delegation, Build Protocol-Level Tooling Only (Chosen)

**Pros:** Ships useful, working commands (`test`, `tools`) that developers and agents can use today against any MCP URL. No dead code paths. No commands that fail against missing APIs. Clean scope boundary.

**Cons:** Doesn't solve the "manage MCP servers from the CLI" need. That need is real, but it's blocked on the platform, not on us.

---

## Consequences

### What Becomes Easier

- **CLI stays fast**: No extra protocol hop for standard commands. `wk recipes list` goes directly to the REST API.
- **Less code to maintain**: No delegation router, no fallback logic, no MCP-specific error handling in the hot path.
- **Clear user mental model**: The CLI talks to the Workato API. Period. MCP commands are diagnostic tools, not an alternative execution path.

### What Becomes Harder

- **MCP server management**: Developers must use the AI Hub UI to create and manage Collection MCP servers. There is no CLI-first workflow for this today.
- **CI/CD MCP provisioning**: Pipelines cannot programmatically create MCP servers. This blocks fully automated MCP-enabled deployment workflows.

### What We'll Need to Revisit

- **When Workato ships MCP management API endpoints**: Implement `wk mcp list/get/create/update/delete` commands. The types (`MCPServerInfo`, `MCPTool`) and MCP client (`internal/mcp/client.go`) are already in place.
- **If the MCP server gains capabilities beyond passthrough**: Re-evaluate delegation. If a future MCP server version adds composite operations, validation, or enriched responses, the `auto_delegate` concept may become worth building. The `[mcp]` config section is already in the schema.
- **`wkt_token` programmatic access**: The single biggest blocker for MCP management is that the `wkt_token` (Collection MCP auth) is only generated through the UI. If this becomes API-accessible, MCP CRUD becomes unblocked even without dedicated MCP management endpoints.

---

## Internal Action Required

Request MCP management API endpoints from the core platform team. The CLI needs at minimum:

```
GET    /api/mcp_servers              → list MCP servers in the workspace
GET    /api/mcp_servers/{id}         → get server details (including wkt_token, collection, URL)
POST   /api/mcp_servers              → create MCP server from a collection
PUT    /api/mcp_servers/{id}         → update server config
DELETE /api/mcp_servers/{id}         → delete MCP server
```

Without these, developers are forced into the AI Hub UI for MCP server management — an unacceptable end-state for a CLI-first workflow. This is a negotiation with core product, not a CLI engineering problem.

---

## Action Items

1. [x] Probe live MCP infrastructure and document findings (2026-02-19)
2. [x] Implement `wk mcp test` with Streamable HTTP transport
3. [x] Implement `wk mcp tools` with initialize + tools/list flow
4. [x] Write unit tests for MCP client (SSE + JSON response handling)
5. [ ] File internal request for MCP management API endpoints with core platform team
6. [ ] When MCP management API ships: implement `wk mcp list/get/create/update/delete`
7. [ ] Update ADR-001 Forward References to mark ADR-003 as Accepted
