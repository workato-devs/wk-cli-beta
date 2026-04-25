# wk Beta Program

## Status

This is a pre-release beta. The CLI is functional for daily use but has not
been through a broad testing cycle. Expect rough edges.

## The toolkit

The full toolkit is the intended workflow. Set up all four components:

1. **wk-cli-beta** — CLI for workspace operations ([install](./README.md#1-install-the-cli))
2. **visualizer-ext-beta** — VS Code extension for visual recipe inspection ([install](./README.md#2-install-the-recipe-visualizer))
3. **wk-lint-beta** — Recipe linter plugin ([install](./README.md#3-install-the-linter-plugin))
4. **recipe-skills** — Connector knowledge for AI agents ([clone](./README.md#4-clone-the-skills-repo))

See the [README](./README.md#setting-up-the-toolkit) for setup instructions.

## What works

See the [ROADMAP](./docs/ROADMAP.md) for detailed status. Summary:

- **Project lifecycle:** init, link, clone, status, pull, push, diff
- **Recipes:** list, get, start, stop, export, import, update, delete, jobs,
  copy, update-connection, validate, versions
- **Connections:** list, get, create, update, delete, disconnect
- **Folders:** list, create, delete
- **Tags:** list, create, update, delete, apply, remove
- **API Platform:** collections (list, create), endpoints (list, enable, disable)
- **MCP:** test server connectivity, list tools
- **Workspace:** info, users, audit-log
- **Connectors:** list with search (read-only)
- **Sync entries:** add, list, refresh, remove
- **Auth:** keychain and file-based credential stores, multi-profile,
  login/list/switch/status/delete
- **Plugins:** install, list, remove, JSON-RPC dispatch, pre-push hooks
- **Output:** `--json` on all commands, text tables, `--verbose`, `--quiet`

## Out of scope for beta

These features are tracked in the [ROADMAP](./docs/ROADMAP.md) but are not
available during the beta period:

- **Auth Tier 1** — secrets manager backends (Vault, AWS Secrets Manager, Doppler)
- **Auth Tier 4** — encrypted file-based credential store
- **Auth rotate** — credential rotation
- **MCP server CRUD** — blocked on missing Workato API endpoints
- **Plugin bidirectional RPC** — plugin-initiated calls back to the CLI host
- **`wk migrate`** — automated migration from the Python CLI
- **`--toml` output format**
- **`--no-color` wiring** — the flag is accepted but has no effect

## Known rough edges

- **`connections test`** (health-check a connection) is not implemented
- **Linter plugin and visualizer extension** are installed separately from the
  CLI — see the [toolkit setup](./README.md#setting-up-the-toolkit)
- **`--no-color`** is accepted on all commands but currently a no-op
- **`post-pull` plugin hook** is not yet implemented (only `pre-push`)

## Production workspace warning

**The CLI can modify your Workato workspace.** Commands like `push`, `recipes
start`, `recipes stop`, `recipes delete`, `connections delete`, and `folders
delete` make real changes. We recommend working in a Developer Sandbox or
non-production workspace during the beta.

The CLI includes a workspace isolation check (`wk.toml` workspace vs. active
profile) that prevents accidental cross-workspace operations, but it is not a
substitute for environment discipline.

## Feedback

- **Issues:** file on the relevant repo ([wk-cli-beta](https://github.com/workato-devs/wk-cli-beta/issues), [wk-lint-beta](https://github.com/workato-devs/wk-lint-beta/issues), [recipe-skills](https://github.com/workato-devs/recipe-skills/issues), [visualizer-ext-beta](https://github.com/workato-devs/visualizer-ext-beta/issues))
- **Slack:** #wk-beta-testers
