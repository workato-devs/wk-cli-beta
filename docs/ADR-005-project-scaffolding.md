# ADR-005: Project Scaffolding — Container Folder, `.wk/` Directory, and Ignore Semantics

**Status:** Accepted
**Date:** March 26, 2026 (revised April 8, 2026; implemented April 17, 2026)
**Author:** Zayne Turner
**Deciders:** DevRel Engineering
**References:** ADR-001 (Foundational Architecture), ADR-002 (Sync Engine), Tester Feedback (Greenfield Project Setup)

> This revision supersedes the April 1, 2026 amendment on sidecar metadata location by integrating those decisions into the main body. It also adds decisions for folder ID caching, init overwrite behavior, and `.wkignore` support.

---

## Context

Tester feedback from the CLI beta identified several gaps between expected and actual behavior when setting up and managing projects:

1. **No container folder.** `wk init` creates `wk.toml` in the current working directory with no named container. `wk clone` creates a named directory. The two commands produce structurally different outcomes for what should be a consistent convention.

2. **Tool-managed files mixed with developer files.** `wk.toml` and `.wk-meta.json` sidecar files sit alongside pushable assets, creating clutter and confusion. Developers encounter files they didn't create and don't understand.

3. **No folder ID caching.** Every `pull`, `push`, and `diff` resolves the remote folder hierarchy via API calls. This is slow and unnecessary after the first resolution.

4. **No overwrite path for `wk init`.** If `wk.toml` already exists, `wk init` hard-errors with no option to reinitialize. CI/CD and agent workflows need a non-interactive overwrite path.

5. **No ignore mechanism.** Developers cannot exclude files from sync. There is no `.wkignore` or equivalent to prevent certain assets from being pulled or pushed.

---

## Decision

Adopt a container folder convention where all tool-managed state — project config, sidecar metadata, and sync state — lives inside a `.wk/` directory at the project root. Add folder ID caching in `wk.toml`, overwrite support for `wk init`, and a `.wkignore` file for user-defined sync exclusions.

---

## Key Design Decisions

### Decision 1: Container Folder Is the Project Root

**Decision:** `wk init --name my-project` creates `my-project/` in the CWD. All tool-managed files live inside `my-project/.wk/`. Asset directories contain only pushable/pullable content.

**Expected structure:**

```
~/workato/                              (developer's CWD)
└── my-project/                         (container = project root)
    ├── .wk/                            (tool-managed — self-ignored via .wk/.gitignore)
    │   ├── wk.toml                     (project config)
    │   ├── meta.json                   (project-level metadata)
    │   ├── recipes/
    │   │   └── my_recipe.meta.json     (remote hash, last push timestamp, asset ID)
    │   └── connections/
    │       └── slack.meta.json
    ├── .gitignore                      (developer-owned; CLI never writes here)
    ├── .wkignore                       (user-defined ignore patterns)
    ├── recipes/                        (created by first pull)
    │   └── my_recipe.recipe.json
    └── connections/                    (created by first pull)
        └── slack.connection.json
```

**Why:** `.wk/` is to `wk` what `.git/` is to `git`. Consolidating all tool-managed state into a single directory means the project root contains only developer-facing files. No sidecar metadata clutters asset directories. A self-ignore file inside `.wk/` (see Decision 8) hides the directory's contents from git without requiring the CLI to modify any file the developer owns.

**Consequence:** `wk init` no longer treats the CWD as the project root. It always creates (or scaffolds into) a child directory. Developers who want the current directory to be the project root should use `wk clone` from the parent, or manually create the directory and run `wk init` from within it (though this is not the recommended workflow).

### Decision 2: `wk init` Creates the Container Directory

**Decision:** `wk init --name my-project` performs the following steps:

1. Check that the CWD is not already inside an existing wk project (see Decision 3)
2. Compute target path: `<CWD>/<project-name>/`
3. If target directory contains `.wk/wk.toml`:
   - **Interactive mode:** Prompt "Project config already exists. Overwrite? [y/N]". Abort on decline.
   - **Non-interactive mode (`--json`):** Error unless `--overwrite` flag is set.
4. If target directory exists but has no `.wk/wk.toml` → scaffold into it (create `.wk/`, add `wk.toml`)
5. If target directory does not exist → create it with `os.MkdirAll`
6. Create `.wk/` directory inside the target
7. Write `wk.toml` inside `.wk/`
8. Write `.wk/.gitignore` with a self-ignore body (`*` plus `!.gitignore`). The project-root `.gitignore` is never touched (see Decision 8).

**Why:** Step 3 replaces the previous hard-error behavior. Interactive prompting gives developers control. The `--overwrite` flag supports CI/CD and agent workflows where interactive prompts are unavailable. Step 4 (scaffold into existing directory) supports the case where a developer has already created a directory or initialized a Git repository before running `wk init`. Step 8 ensures tool-managed files never leak into version control.

**Implementation:** `internal/commands/init.go` — Replace the CWD-based config path with `filepath.Join(cwd, name, ".wk", "wk.toml")` as the config path. Create `.wk/` directory before saving config. Add `--overwrite` flag.

### Decision 3: Error When Already Inside a Project

**Decision:** If `FindProjectRoot(cwd)` successfully locates a `.wk/wk.toml` in the CWD or any parent directory, `wk init` exits with an error:

```
Error: already inside wk project at /path/to/my-project/.wk/wk.toml
Run from outside the project directory.
```

**Why:** Nested wk projects would create ambiguity for every other command (`pull`, `push`, `status`, `diff`) because `FindProjectRoot` walks upward and stops at the first `.wk/wk.toml` it finds. A nested project's config would shadow the parent, or vice versa depending on the developer's CWD. Rather than introducing complex resolution logic, prevent nesting entirely.

**Consequence:** Developers who genuinely need multiple wk projects in the same tree must keep them as siblings, not nested. This matches how Git repositories work — nested `.git` directories are an antipattern.

### Decision 4: `wk clone` Alignment

**Decision:** `wk clone` aligns with `wk init` by creating the same `.wk/` directory structure:

- Creates `<local-path>/.wk/` and writes `wk.toml` inside
- Writes `folder_id` to the sync entry immediately (since clone resolves the folder during setup)
- Writes `.wk/.gitignore` (self-ignore; see Decision 8)
- Checks that the CWD is not already inside an existing project (same nesting guard as init)
- The `Name` field in `wk.toml` matches the container folder name

**Why:** Both commands should produce the same structural outcome. A developer should be able to look at a project directory and not know whether it was created by `init + pull` or by `clone`.

**Implementation:** `internal/commands/clone.go` — Add the `FindProjectRoot` guard. Write config to `.wk/wk.toml`. Ensure `cfg.Name` is set from the directory name, not just the server folder name (they may differ if `--local-path` is used).

### Decision 5: `FindProjectRoot` Changes

**Decision:** `FindProjectRoot` walks upward looking for `.wk/wk.toml` instead of a bare `wk.toml`. The function returns the **parent** of `.wk/` (i.e., the project root directory), not the `.wk/` directory itself.

Introduce a `ProjectDir` constant alongside the existing `ProjectFile`:

```go
const ProjectDir  = ".wk"
const ProjectFile = "wk.toml"
```

`FindProjectRoot` changes from:
```go
configPath := filepath.Join(dir, ProjectFile)
```
to:
```go
configPath := filepath.Join(dir, ProjectDir, ProjectFile)
```

**Why:** The project root is the container directory — the directory developers `cd` into, where asset folders and `.wkignore` live. `.wk/` is an implementation detail inside the project root, not the root itself. Returning the parent of `.wk/` means all relative paths (`local_path` in sync entries, `.wkignore` pattern evaluation) resolve naturally from the project root.

**Impact:** Every call site that currently does `filepath.Join(projectRoot, config.ProjectFile)` must change to `filepath.Join(projectRoot, config.ProjectDir, config.ProjectFile)`. This is a pervasive but mechanical change affecting `init.go`, `clone.go`, `link.go`, and `root.go`'s `BuildRunContext`.

### Decision 6: Sync `local_path` Semantics

**Decision:** `local_path` in `[[sync]]` entries is relative to the project root (the parent of `.wk/`). With the container convention, this means relative to the container folder. The default `local_path` for init should be `"."` (same as clone), meaning artifacts are extracted directly into the project root.

**Why:** This makes `wk init --name my-project --server-path "Recipes/Prod"` followed by `wk pull` produce the same layout as `wk clone "Recipes/Prod" --local-path my-project`. Consistency between the two workflows.

**Current behavior (init):** `local_path` defaults to `./<leaf of server path>` (e.g., `./Prod`). This creates an extra nesting level inside the container that diverges from clone behavior.

**New behavior (init):** `local_path` defaults to `"."` to match clone. Developers who want subdirectory nesting can still set `--local-path` explicitly.

### Decision 7: Inner Directory Layout Is Not Pre-Scaffolded

**Decision:** `wk init` creates `.wk/` (because it must contain `wk.toml`), but does not pre-create subdirectories like `recipes/`, `connections/`, or `.wk/recipes/`. These are created by the sync engine when assets are first pulled or pushed.

**Why:** The inner directory structure depends on what assets exist on the server. Pre-creating empty directories would be misleading (implying assets exist when they don't) and would need to be kept in sync with the server's asset types. The RLCM zip extraction already handles directory creation correctly. The same applies to metadata subdirectories within `.wk/` — they are created when metadata is first written.

### Decision 8: `.wk/` Self-Ignores via `.wk/.gitignore`

**Decision:** `wk init` and `wk clone` write a self-ignore file at `<project-root>/.wk/.gitignore` that hides every tool-managed file from git. The project-root `.gitignore` is never touched — that file belongs to the developer. A project may legitimately end up with two `.gitignore` files (the developer's at the root and the CLI's inside `.wk/`); that is intentional.

The fixed contents of `<project-root>/.wk/.gitignore`:

```
# wk CLI — tool-managed directory. Contents are machine-local state
# (project config, sidecar metadata, folder-ID cache). Do not commit.
*
!.gitignore
```

**Why this structure:**

- **`*`** hides every other file in `.wk/` from git, so `wk.toml`, sidecar metadata, and the folder-ID cache never appear as untracked state to the developer. This works regardless of whether the developer has listed `.wk/` in their own project-root `.gitignore`.
- **`!.gitignore`** un-ignores this file itself, keeping it visible to `git status` and committable if the team wants a consistent ignore policy checked into the repo.

**Why write-only-under-`.wk/`:**

- **`wk.toml` contains environment-specific references.** The `workspace` field references a local auth profile. Different developers may use different profiles (personal tokens, team tokens, staging vs. production). Committing `wk.toml` would force a shared workspace identity or create constant merge conflicts.
- **`folder_id` values are environment-specific.** Folder IDs may differ between Workato environments (staging workspace vs. production workspace). A committed `folder_id` from one developer's workspace would be meaningless to another.
- **Sidecar metadata is machine-local.** Content hashes, pull timestamps, and zip entry names are per-developer state used for diff tracking. They have no meaning outside the machine that produced them.
- **The developer's project-root `.gitignore` is theirs.** Mutating a file the developer maintains (adding lines to a file that might be under their own version control, code review, or tooling) is overreach. Keeping the CLI's state behind a self-ignore file inside its own directory avoids that coupling entirely.

**Precedent:** the `*` + `!.gitignore` idiom is the standard pattern used by per-directory ignore files in Rails (`log/.gitignore`, `tmp/.gitignore`), Go build systems (`testdata/.gitignore` in various projects), and similar tool-managed directories. This is a deliberate departure from `cargo new`'s and `npm init`'s practice of appending to the project-root `.gitignore` — those tools mutate a file developers own; we chose not to.

### Decision 9: Remote Folder IDs in `wk.toml`

**Decision:** Add a `folder_id` field to `SyncEntry` that caches the resolved Workato folder ID:

```toml
[[sync]]
server_path = "All projects/Recipes/Production"
local_path = "."
folder_id = 12345
```

**Behavior:**

- **`wk pull` / `wk push` / `wk diff`:** If `folder_id` is set and non-zero on the sync entry, use it directly — skip the `resolveFolderID` API call. If it is zero or absent, resolve via the folder hierarchy API and write the resolved ID back to `wk.toml` (write-through cache).
- **`wk init`:** `folder_id` starts as 0 (omitted from TOML via `omitempty`). It is populated on the first pull or push.
- **`wk clone`:** Writes `folder_id` immediately since clone resolves the folder during setup.
- **Invalidation:** If the API returns a 404 or "folder not found" for a cached `folder_id`, fall back to `resolveFolderID` by path, update the cached ID in `wk.toml`, and retry the operation.

**Why:** `resolveFolderID` walks the folder hierarchy via multiple API calls (one `GET /folders` per path segment). For a three-level path, that is three sequential API calls on every pull/push. Caching the resolved ID eliminates this overhead after the first sync. The write-through pattern means developers never need to manually populate `folder_id` — it is resolved and cached automatically.

**Config struct change:**

```go
type SyncEntry struct {
    ServerPath string   `toml:"server_path"`
    LocalPath  string   `toml:"local_path"`
    FolderID   int      `toml:"folder_id,omitempty"`
    Include    []string `toml:"include,omitempty"`
}
```

### Decision 10: `.wkignore` File

**Decision:** Support a `.wkignore` file at the project root (sibling to `.wk/`, outside it) that specifies patterns the CLI should skip during push and pull operations.

**Location:** `<project-root>/.wkignore`. This file is version-controlled (it lives outside `.wk/`) so that team members share the same ignore rules.

**Format:** Gitignore-style glob patterns.

- One pattern per line. Blank lines and lines starting with `#` are ignored.
- Patterns follow gitignore semantics: `*` matches within a single path component, `**` matches across path separators, a trailing `/` matches only directories, a leading `!` negates a previous pattern.
- Patterns are evaluated relative to the project root.
- `.wk/` is implicitly ignored by the sync engine regardless of `.wkignore` content.
- If `.wkignore` does not exist, no patterns are applied (default: include everything).

**Example `.wkignore`:**

```
# Ignore test fixtures
test_fixtures/

# Ignore backup files
*.bak
*.tmp
```

**Scope:** `.wkignore` affects `push`, `pull`, `status`, and `diff`. It does NOT affect what the Workato API exports — it only controls what the CLI writes locally (pull) or includes in the upload zip (push).

**Why gitignore format:**

1. **Developer familiarity.** Every developer who uses Git knows gitignore syntax. Zero learning curve.
2. **Agent familiarity.** AI agents have extensive training data on `.gitignore` files. Asking an agent to "add `*.tmp` to `.wkignore`" works immediately.
3. **Directory pruning.** Patterns with a trailing `/` enable `filepath.SkipDir` to prune entire subtrees during tree walking, avoiding unnecessary I/O.
4. **Library support.** Go has mature gitignore-pattern libraries (e.g., `go-gitignore`, `doublestar`). No custom parser required.
5. **Comments and negation.** Developers can document why patterns exist (`# Ignore test fixtures`) and selectively un-ignore files (`!recipes/keep_this.recipe.json`). A TOML list or YAML array cannot express these.

**Implementation:** New file `internal/sync/ignore.go` — loads `.wkignore` from project root, compiles patterns, exposes a `Match(path string) bool` function. The `filepath.Walk` calls in `status.go`, `pull.go`, `push.go`, and `diff.go` check each path against the compiled patterns.

---

## Impact on Existing Commands

| Command | Change Required | Details |
|---------|----------------|---------|
| `wk init` | **Yes** | Create `.wk/` dir, write config inside, write `.wk/.gitignore`, overwrite prompt/flag, nesting guard |
| `wk clone` | **Yes** | Same `.wk/` scaffolding as init, write `folder_id`, nesting guard |
| `wk pull` | **Yes** | Read metadata from `.wk/`, respect `.wkignore`, use cached `folder_id` |
| `wk push` | **Yes** | Read metadata from `.wk/`, respect `.wkignore`, use cached `folder_id` |
| `wk status` | **Yes** | Read metadata from `.wk/`, respect `.wkignore` |
| `wk diff` | **Yes** | Read metadata from `.wk/`, respect `.wkignore`, use cached `folder_id` |
| `wk link` | **Minor** | Config path changes to `.wk/wk.toml` |
| `FindProjectRoot` | **Yes** | Walk up looking for `.wk/wk.toml` instead of bare `wk.toml` |

---

## Consequences

### What Becomes Easier

- **Greenfield setup**: Developers get a clean, named project directory with a single command. No need to manually create directories or reason about where config should live.
- **Multi-project workspaces**: Developers can have multiple wk projects as siblings in the same parent directory, each in its own named container.
- **Mental model**: "The project name is the folder name" and "`.wk/` is the tool directory" are simple, match established conventions (Git, Cargo, npm).
- **Clean asset directories**: No sidecar metadata files appear alongside developer code. Asset directories contain only pushable/pullable content.
- **Fast folder resolution**: After the first sync, `folder_id` is cached in `wk.toml`. Subsequent pull/push operations skip the multi-call folder hierarchy walk.
- **Shared ignore rules**: `.wkignore` is version-controlled, ensuring consistent sync behavior across team members and CI.
- **Gitignore is foolproof**: One entry (`/.wk/`) covers all current and future tool-managed files.

### What Becomes Harder

- **In-place initialization**: Developers who want the CWD itself to be the project root (no container subdirectory) can no longer do this with `wk init`. This workflow is uncommon and can be worked around by creating the directory manually.
- **Debugging metadata**: Inspecting sidecar metadata requires navigating into `.wk/` rather than seeing it next to the asset. Mitigated by `wk status` showing metadata state.
- **Parallel directory maintenance**: The CLI must maintain the parallel directory structure inside `.wk/` when assets are moved or renamed.
- **Pervasive `FindProjectRoot` change**: Every command that loads config must update its path from `projectRoot/wk.toml` to `projectRoot/.wk/wk.toml`. This is mechanical but touches many files.

### Migration

This is a **breaking change** for any existing projects initialized with the current `wk init` behavior (where `wk.toml` lives at the project root without a `.wk/` directory). Since the CLI is in beta, this is acceptable. Existing projects can be migrated by:

1. Creating the `.wk/` directory in the project root
2. Moving `wk.toml` from the project root into `.wk/`
3. Moving all `.wk-meta.json` sidecar files into the `.wk/` mirror structure (strip the `.wk-meta.json` suffix, rewrite as `.meta.json` under `.wk/<relative-path>/`)
4. Writing a `.wk/.gitignore` self-ignore file (the CLI no longer modifies the project-root `.gitignore`)
5. Removing old `.wk-meta.json` files from asset directories
6. All relative paths in `wk.toml` remain valid (they are relative to the project root, which is the parent of `.wk/`)

This migration can be automated via a `wk migrate` command (see Action Items).

---

## Action Items

1. [x] Update `internal/config/config.go` — add `ProjectDir` constant (`.wk`), update `FindProjectRoot` to look for `.wk/wk.toml`
2. [x] Add `FolderID int` field to `SyncEntry` struct with `toml:"folder_id,omitempty"`
3. [x] Update `internal/commands/init.go` — create `.wk/` dir, write config inside, write `.gitignore`, add `--overwrite` flag, add interactive overwrite prompt, add nesting guard, default `local_path` to `"."`
4. [x] Update `internal/commands/clone.go` — same `.wk/` scaffolding, write `folder_id` (via engine write-through), add nesting guard
5. [x] Update `internal/commands/link.go` — config path now under `.wk/`
6. [x] Update `internal/sync/helpers.go` — `folderIDForEntry` fast path when `FolderID` is set, write-through cache to config, invalidation + retry on API 404
7. [x] Implement `.wkignore` parser in new file `internal/sync/ignore.go` — hand-rolled gitignore-subset matcher (no new dependency) covering `*`, `**`, trailing `/`, `!` negation, comments, blanks
8. [x] Update `internal/sync/status.go`, `pull.go`, `push.go`, `diff.go` to respect `.wkignore` patterns and implicitly skip `.wk/`
9. [x] Update `internal/sync/meta.go` — metadata path derivation to `<root>/.wk/<relative-asset-path>.meta.json`; removed co-located `.wk-meta.json` sidecar logic
10. [x] Update all tests (`config_test.go`, `init_test.go`, `meta_test.go`, `status_test.go`, `helpers_test.go`, `auth_test.go`, `resolve_test.go`); added `ignore_test.go`, folder-ID cache tests, overwrite / `.gitignore` tests
11. [x] Update CLI help text for `wk init`, `wk clone`, `wk link`
12. [ ] ~~Implement `wk migrate` command for existing beta projects~~ — **Deferred.** CLI is in beta with no within-CLI backward-compat guarantees; developers re-init rather than migrate. Revisit if the beta cohort grows large enough that manual re-init becomes a burden.

## Implementation Notes

- **`.wkignore` matcher is hand-rolled**, not backed by a third-party gitignore library. The subset documented in Decision 10 is narrow enough that owning the matcher keeps our semantics stable and the dependency list minimal. Anything outside that subset (leading-slash anchoring beyond the default, re-inclusion after parent exclusion, character classes) is intentionally out of scope.
- **Folder-ID cache invalidation.** The write-through cache writes the resolved ID back to `wk.toml` on first resolution. If a subsequent `Export` / `Import` / `fetchRemoteFiles` returns `ErrAPINotFound` and the call used a cached ID, the engine re-resolves via path, updates the cache, and retries once. Non-404 errors surface unchanged.
- **Meta file naming.** Assets at `<root>/<rel-path>` have their metas at `<root>/.wk/<rel-path>.meta.json` (full asset filename preserved, `.meta.json` appended). This matches the migration spec in Decision 4 and avoids collisions between assets that share a stem.
- **`profiles.env` placement.** Lives at `<project-root>/.wk/profiles.env` — alongside `wk.toml` inside the tool-managed directory (ADR-006 Sub-decision 3). This was a latent bug for one revision: the original ADR-006 said "alongside `wk.toml`," but when ADR-005 moved `wk.toml` into `.wk/`, the code that resolved `profiles.env` kept the project-root path. The fix restores the design intent and has a safety benefit — `profiles.env` holds API tokens and must never be committed, so living under the self-ignored `.wk/` (Decision 8) is strictly better than sitting at the project root where the developer would have to remember to `.gitignore` it themselves.

## Related: Issue #29 — `init` sync-entry ergonomics

Issue #29 observes that `wk init`'s flag surface expresses at most one `[[sync]]` entry, which forces developers with multi-folder projects into hand-editing `wk.toml`. Two interactions with ADR-005 matter:

1. **`--overwrite` data-loss footgun (addressed).** Decision 2 added `--overwrite` as a non-interactive escape hatch. The initial implementation rewrote `wk.toml` from the command-line flags alone, which would have silently discarded any hand-edited `[[sync]]` blocks — including the multi-entry workaround teams are using today. The command now **loads the existing config before overwrite and preserves its `Sync` entries**, appending any `--server-path` flag as a new entry (deduplicated on `ServerPath`+`LocalPath`). A stderr notice reports how many entries were preserved. Non-sync fields (`Name`, `Profile`, workspace/environment/email snapshots) still get replaced from flags — that's the intended scope of `--overwrite`.

2. **Incremental sync management (deferred).** A proper `wk sync add` / `wk sync list` / `wk sync remove` subcommand tree is the right long-term home for multi-entry scaffolding, including per-entry `--verify`. Deferred from this ADR to a follow-up (tracked in issue #29). The folder-ID cache in Decision 9 composes cleanly with incremental adds — a new entry starts with `FolderID=0` and is resolved + cached on its first sync.

Combined, these keep the current init surface simple while eliminating the destructive `--overwrite` semantics that would have made multi-entry hand-editing unsafe.
