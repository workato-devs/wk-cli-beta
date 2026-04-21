# Changelog

All notable changes to `wk` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.0-beta] - 2026-04-21

Initial beta release. See [ROADMAP.md](./docs/ROADMAP.md) for detailed feature
status and remaining work.

### Added

- Project lifecycle: init, link, clone, status, pull, push, diff
- Recipe management: list, get, start, stop, export, import, update, delete,
  jobs, copy, update-connection, validate, versions
- Connection management: list, get, create, update, delete, disconnect
- Folder management: list, create, delete
- Tag management: list, create, update, delete, apply, remove
- API Platform: collections (list, create), endpoints (list, enable, disable)
- MCP: test server connectivity, list tools
- Workspace: info, users, audit-log
- Connectors: list with search
- Sync entry management: add, list, refresh, remove
- Auth: keychain and file-based credential stores, multi-profile management
- Plugin system: install, list, remove, JSON-RPC dispatch, pre-push hooks
- `--json` output on all commands
- Workspace isolation check (prevents cross-workspace operations)
- CI/CD support with file-based credential store (profiles.env)
- Cross-platform binaries: linux/darwin/windows on amd64/arm64
