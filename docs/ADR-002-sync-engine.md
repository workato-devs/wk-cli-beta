# ADR-002: Sync Engine — RLCM-Based Pull/Push with Sidecar Metadata

**Status:** Accepted
**Date:** March 3, 2026
**Author:** Zayne Turner
**Deciders:** DevRel Engineering
**References:** ADR-001 (Foundational Architecture), PRD Section "Sync Engine Design"

---

## Context

The `wk` CLI needs to synchronize recipe and connector code between a developer's local filesystem and the Workato platform. Workato's API does not expose individual recipe files as first-class resources that can be read or written directly — it exposes Recipe Lifecycle Management (RLCM) operations: export manifests, package exports (zip archives), and package imports. Any sync engine for the CLI must work within these constraints.

The PRD proposed a sync engine with pull, push, status, and diff commands. During implementation, we discovered several design decisions that the PRD left open or that required adaptation to the realities of the RLCM API. This ADR documents the sync engine architecture as built, the decisions made during implementation, and the trade-offs accepted.

---

## Decision

Build a sync engine around RLCM export/import operations with `.wk-meta.json` sidecar files tracking server-side identity, SHA256 content hashing for local change detection, and folder resolution by walking the Workato hierarchy. Status computation is local-only (no API calls). Remote diff requires an export round-trip.

---

## Architecture Overview

### Sync Flow: Pull

```
wk pull
  → Check for local modifications (conflict detection)
  → Resolve server_path to Workato folder ID (hierarchy walk)
  → Create RLCM export manifest (POST /export_manifests)
  → Trigger package export (POST /packages/export/{id})
  → Poll until export completes (GET /packages/export/{id}, 2s interval)
  → Download zip archive
  → Extract files to local_path
  → Write .wk-meta.json sidecar for each extracted file
```

### Sync Flow: Push

```
wk push
  → Compute local status (hash comparison against meta)
  → Filter to changed/new files only
  → Build zip archive (using zip_name from meta for path identity)
  → Trigger RLCM import (POST /packages/import/{folder_id}?restart_recipes={bool})
  → Poll until import completes (GET /packages/import/{id}, 2s interval)
  → Run plugin pre-push hooks (if not --skip-hooks)
  → Update .wk-meta.json with new content hashes
```

### Sync Flow: Status (Local-Only)

```
wk status
  → Find all .wk-meta.json files under local_path
  → For each tracked file: hash current content, compare to stored content_hash
  → Report: unchanged | modified | new | deleted
  → No API calls
```

### Sync Flow: Diff (Remote Comparison)

```
wk diff
  → Export remote folder (same as pull export flow)
  → Hash each file in the remote zip
  → Hash each local file
  → Compare: added | modified | deleted | same
```

---

## Key Design Decisions

### Decision 1: RLCM as the Sync Transport

**Decision:** All sync operations go through the RLCM export/import API. The CLI never reads or writes individual recipes via the REST API.

**Why:** Workato's API models recipes as components within packages, not as standalone files. The RLCM API is the only supported mechanism for moving recipe code between workspaces (or between a workspace and a local filesystem). It handles dependency resolution, connection mapping, and recipe bundling. Attempting to build file-level sync on top of the single-recipe REST endpoints would bypass these guarantees and produce packages that can't be reliably imported.

**Consequence:** Every pull and push involves a zip archive. There is no way to sync a single file — the smallest unit of sync is whatever the RLCM export manifest produces for a given folder path. This is a known trade-off: correctness over granularity.

**Implementation:** `internal/api/packages.go` wraps the full RLCM lifecycle — `Export()`, `ExportStatus()`, `Download()`, `Import()`, and `ImportStatus()` for polling.

### Decision 2: `.wk-meta.json` Sidecar Files for Path Identity

**Decision:** Each synced file gets a `.wk-meta.json` sidecar file stored alongside it. The sidecar tracks the file's server-side identity and sync state.

**Schema:**

```json
{
  "server_path": "Demos/Slack Bot/Recipe 1",
  "zip_name": "recipe_1.json",
  "folder": "Demos/Slack Bot",
  "type": "recipe",
  "content_hash": "sha256:a1b2c3...",
  "last_pulled_at": "2026-02-19T10:30:00Z"
}
```

**Why:** The RLCM zip archive uses its own internal naming conventions (`zip_name`) that don't match the human-readable file names a developer sees locally. When pushing, the CLI must reconstruct a zip that the RLCM import endpoint will accept — which means knowing the `zip_name` for each file. The sidecar stores this mapping.

The `server_path` field tracks where the file lives in the Workato folder hierarchy. This is critical because local file paths can be reorganized by the developer without affecting the server-side identity. The `content_hash` enables local-only status checks without calling the API. (The PRD proposed a `version` field for merge-base resolution; this was deferred from the initial implementation but can be added when bidirectional merge is needed.)

**Alternative considered:** Store all metadata in a single `.wk-meta.json` at the project root. Rejected because per-file sidecars survive file moves, renames, and partial syncs without requiring a global index rebuild.

**Implementation:** `internal/sync/meta.go` — `AssetMeta` struct with `ServerPath`, `ZipName`, `Folder`, `Type`, `ContentHash`, `LastPulledAt` fields. `ReadMeta`/`WriteMeta` for individual files, `FindMetaFiles` for directory scanning.

### Decision 3: SHA256 Content Hashing for Change Detection

**Decision:** Use SHA256 hashes of file content to detect local modifications. Store the hash in `.wk-meta.json` at pull time. Compare at status/push time.

**Why:** Timestamp-based change detection (mtime) is unreliable across filesystems, Git operations (checkout resets mtime), and CI environments. Content hashing is deterministic: if the hash matches, the file hasn't changed, regardless of what the filesystem reports about modification time.

SHA256 was chosen over faster alternatives (xxHash, CRC32) because it's available in Go's standard library with no external dependencies and the files being hashed (recipe JSON) are small enough that hashing speed is irrelevant.

**Implementation:** `internal/sync/meta.go` — `ComputeHash()` computes `sha256:` prefixed hex digest. `internal/sync/status.go` — `Status()` compares current hash against stored `content_hash`, returning `[]AssetStatus` entries classified as `StatusUnchanged`, `StatusModified`, `StatusNew`, or `StatusDeleted`.

### Decision 4: Folder Resolution by Hierarchy Walk

**Decision:** Resolve a `server_path` like `"Demos/Slack Bot"` to a Workato folder ID by walking the folder hierarchy from root, matching each path segment. Strip the `"All projects"` prefix if present.

**Why:** Workato's folder API doesn't support lookup-by-path. Folders are identified by numeric IDs, and the only way to resolve a human-readable path to an ID is to list children at each level and match by name. The `"All projects"` prefix is a UI construct — the API's root folder is the workspace root, not "All projects." Developers who copy paths from the Workato UI will include this prefix, so the CLI strips it silently.

**Implementation:** `internal/sync/helpers.go` — `resolveFolderID()` takes a path string and an API client, splits on `/`, strips `"All projects"` if present, and walks the tree. Returns an error if any segment doesn't match.

**Test coverage:** `internal/sync/helpers_test.go` — Tests for prefix stripping, nested path resolution, nonexistent folder errors, and empty path handling.

### Decision 5: Local-Only Status Computation

**Decision:** `wk status` computes file status entirely from local data — `.wk-meta.json` hashes versus current file hashes. It makes zero API calls.

**Why:** Status is the most frequently run sync command. Developers run it reflexively before pull/push, often in rapid succession. Making an API call on every status check would be slow, rate-limited, and unnecessary. The common case is "has anything changed locally since my last pull?" — which is answerable purely from local hashes.

The trade-off is that `wk status` cannot tell you what changed on the server. That's what `wk diff` is for, and diff explicitly requires an API round-trip (a full export to compare against).

**Implementation:** `internal/sync/status.go` — `Status()` returns `[]AssetStatus` entries with `FilePath`, `Status` (unchanged/modified/new/deleted), and metadata context.

### Decision 6: Remote Diff via Export + Hash Comparison

**Decision:** `wk diff` exports the remote folder's current state as a zip, hashes each file in the zip, and compares against local hashes. It reports `added`, `modified`, `deleted`, and `same` entries.

**Why:** There is no incremental change API in Workato. The platform doesn't expose "what changed since timestamp X" for recipes. The only way to know the remote state is to export it. This is expensive (a full export round-trip for every diff), but it's the only correct approach given the API constraints.

**Implementation:** `internal/sync/diff.go` — `Diff()` returns `[]DiffEntry` with name and `DiffType` (`DiffAdded`, `DiffModified`, `DiffDeleted`, `DiffSame`).

### Decision 7: `--preserve-state` Maps to `restart_recipes` on Import

**Decision:** The `--preserve-state` flag (default: `true`) controls whether recipes are restarted after import. It maps directly to the `restart_recipes` query parameter on the RLCM import endpoint.

**Why:** The PRD called this flag `--preserve-state` rather than `--restart-recipes` because the developer's mental model is "I want to push code changes without disrupting running recipes." The underlying API parameter is `restart_recipes` (a boolean). The default is `true` because the common case for a developer push is "deploy and activate" — if you're pushing code, you usually want it running.

**Implementation:** `internal/commands/sync.go` — Push command registers `--preserve-state` as a persistent flag with default `true`. `internal/sync/push.go` — `Push()` accepts `preserveState` as a parameter and passes it through to `internal/api/packages.go` — `Import()` appends `?restart_recipes=true|false` to the import URL.

### Decision 8: Plugin Pre-Push Hooks with Fail-Open Semantics

**Decision:** Before push, the CLI runs pre-push hooks registered by JSON-RPC plugins. Hooks receive the list of files being pushed and can return diagnostics (warnings, errors). If the hook call fails (plugin crashes, timeout, connection error), the push proceeds anyway — fail-open.

**Why:** Plugins are third-party code that the CLI doesn't control. A plugin bug or a slow network connection to a plugin process should not block a developer's push. The hook system is advisory: plugins can warn about lint violations or policy failures, but the CLI never gates a push on plugin health.

The fail-open decision is deliberate and documented. Fail-closed would make the plugin system a reliability liability — every plugin installation becomes a potential push blocker.

**Implementation:** `internal/plugin/hooks.go` — `RunPrePushHook()` iterates registered plugins, sends `HookParams` (containing `[]HookFile` with path, content hash, action), and collects `HookResult` with `[]Diagnostic` (severity, message, file, line). `internal/commands/sync.go` — Push command runs hooks unless `--skip-hooks` is passed.

### Decision 9: Conflict Detection Before Pull

**Decision:** Before pulling, the CLI checks for local modifications that would be overwritten. If modified files exist and `--force` is not set, pull aborts with an `ErrSyncConflict` error listing the conflicting files.

**Why:** A pull overwrites local files with the server's version. If the developer has uncommitted local changes, those changes would be silently destroyed. The conflict check is a safety net — it forces developers to either commit/stash their changes or explicitly pass `--force` to acknowledge the overwrite.

**Implementation:** `internal/sync/pull.go` — `Pull()` calls `Status()` first and checks for any `StatusModified` entries when `force=false`. `internal/errors/errors.go` — `ErrSyncConflict` sentinel error.

### Decision 10: Clone as Pull with Project Scaffolding

**Decision:** `wk clone` creates a new directory, generates a `wk.toml` with a `[[sync]]` entry mapping the server path to the local path, and runs a force-pull. It is not a separate sync mode — it's project initialization followed by a standard pull.

**Why:** Clone is the "first pull" — the developer doesn't have a local project yet. By decomposing clone into scaffolding + pull, we avoid duplicating the pull logic. The `--force` flag is implicit in clone because there are no local files to conflict with.

**Implementation:** `internal/commands/clone.go` — Creates directory, writes `wk.toml`, calls pull with `force=true`.

---

## What Stays from the PRD

| PRD Feature | Status | Notes |
|-------------|--------|-------|
| `wk pull` | **Implemented** | Full RLCM export flow with conflict detection |
| `wk push` | **Implemented** | RLCM import with `--preserve-state`, `--dry-run`, `--skip-hooks` |
| `wk status` | **Implemented** | Local-only hash comparison |
| `wk diff` | **Implemented** | Remote export + hash comparison |
| `wk clone` | **Implemented** | Scaffolding + force-pull |
| `[[sync]]` config entries | **Implemented** | `server_path` + `local_path` mapping in `wk.toml` |
| `.wk-meta.json` sidecars | **Implemented** | Per-file server identity and hash tracking |
| Plugin pre-push hooks | **Implemented** | Fail-open JSON-RPC hooks |

## What Diverged from the PRD

| PRD Expectation | Actual Implementation | Reason |
|-----------------|----------------------|--------|
| Granular file-level sync | Folder-level RLCM packages | RLCM API doesn't support single-file export/import |
| Incremental remote diff | Full export for every diff | No incremental change API exists |
| `--preserve-state` = don't restart | `--preserve-state` default `true` = restart recipes | Developer expectation: push means deploy and activate |
| Status includes remote state | Status is local-only | Performance: status must be instant, no API round-trip |

---

## Consequences

### What Becomes Easier

- **Offline status checks**: `wk status` works without network access or API credentials. Developers can check what they've changed at any time.
- **Deterministic change detection**: SHA256 hashing eliminates false positives from filesystem timestamp drift, Git checkouts, or CI environments.
- **Safe pulls**: Conflict detection prevents accidental overwrites of local work.
- **Plugin extensibility**: Pre-push hooks let teams add linting, policy checks, or custom validation without forking the CLI.

### What Becomes Harder

- **Single-file sync**: Pushing one recipe change requires exporting/importing the entire folder's package. This is a known limitation of the RLCM API, not a CLI design choice.
- **Remote change awareness**: Developers must explicitly run `wk diff` to see server-side changes. There is no ambient notification of remote modifications.
- **Large workspace performance**: Folder resolution walks the hierarchy one level at a time. Deeply nested workspaces pay a proportional API cost. Caching folder IDs would mitigate this but is not yet implemented.

### What We'll Need to Revisit

- **Folder ID caching**: If developers report slow pulls on deep folder hierarchies, add a local cache mapping `server_path` → `folder_id` with a TTL or invalidation on pull.
- **Partial push**: If Workato adds single-recipe import endpoints, the sync engine can be extended to push individual files instead of full packages.
- **Bidirectional merge**: The current model is "last writer wins" with conflict detection. If collaborative workflows require three-way merge, the `.wk-meta.json` schema's `last_pulled_at` field and a future `version` field could support merge base resolution.

---

## Sentinel Errors

The sync engine defines specific error types for programmatic handling:

| Error | Trigger | Behavior |
|-------|---------|----------|
| `ErrSyncConflict` | Local modifications detected before pull | Pull aborts unless `--force` |
| `ErrNoSyncEntries` | No `[[sync]]` entries in `wk.toml` | All sync commands abort with guidance |
| `ErrMetaCorrupted` | `.wk-meta.json` cannot be parsed | Status/push skip the file with warning |
| `ErrProfileMismatch` | Active profile doesn't match the workspace that created the meta | Push aborts to prevent cross-workspace sync |

**Implementation:** `internal/errors/errors.go`

---

## Polling Strategy

Both export and import operations are asynchronous. The CLI polls for completion:

- **Interval:** 2 seconds between polls
- **Mechanism:** `GET /packages/export/{id}` or `GET /packages/import/{id}`
- **Completion signals:** Status field transitions to a terminal state
- **No timeout cap:** The CLI polls indefinitely. Large workspaces can take minutes to export. A hard timeout would create false failures that are worse than waiting.

**Implementation:** `internal/sync/helpers.go` — `waitForPackage()` and `waitForImport()`.

---

## Action Items

1. [x] Implement pull flow with RLCM export, conflict detection, and meta sidecars
2. [x] Implement push flow with RLCM import, `--preserve-state`, and hook integration
3. [x] Implement local-only status computation with SHA256 hashing
4. [x] Implement remote diff via export + hash comparison
5. [x] Implement clone as scaffolding + force-pull
6. [x] Write unit tests for folder resolution, meta read/write, and status computation
7. [ ] Add folder ID caching if hierarchy walk latency becomes a user complaint
8. [ ] Evaluate partial push when/if single-recipe import API becomes available
