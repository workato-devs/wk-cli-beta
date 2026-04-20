# ADR-007: Greenfield Project Onboarding — Sync Entry Lifecycle and Push-Create-on-Demand

**Status:** Accepted
**Date:** April 18, 2026 (revised April 20, 2026; implemented April 20, 2026)
**Author:** Zayne Turner
**Deciders:** DevRel Engineering
**References:** ADR-002 (Sync Engine), ADR-005 (Project Scaffolding), ADR-006 (Profile Identity Model), Issue #29 (init sync-entry ergonomics), dewy-resort demo repo

---

## Context

Two flows motivate this ADR:

**Greenfield onboarding** — the first developer on a team clones a template repo like dewy-resort (or starts from scratch), runs `wk init`, and expects `wk push` to publish their local recipes to a Workato workspace. Nothing exists server-side yet.

**Adoption / re-hydration** — every subsequent developer on the team, every fresh clone, and every recovery from a `.wk/` wipe runs `wk init` against local folders whose server-side counterparts already exist (created by the first developer's push). This is the predominant day-to-day state of the CLI once a team has adopted it — a direct consequence of `wk.toml` being gitignored as machine-local state per ADR-005 Decision 8. The CLI must serve this flow at least as well as greenfield.

Both flows use the same `wk init` + `wk push` commands but have different expectations about server state. The developer picks between them implicitly via `--verify` (adoption: expected to exist) or its absence (greenfield: will be created). No mode flag is required.

Current CLI behavior requires nine commands to scaffold a four-project demo:

```
wk init --name dewy-resort --profile dev \
  --sync "atomic-salesforce-recipes:./workato/recipes/atomic-salesforce-recipes" \
  --sync "atomic-stripe-recipes:./workato/recipes/atomic-stripe-recipes" \
  --sync "home-assistant:./workato/recipes/home-assistant" \
  --sync "orchestrator-recipes:./workato/recipes/orchestrator-recipes"
wk folders create atomic-salesforce-recipes
wk folders create atomic-stripe-recipes
wk folders create home-assistant
wk folders create orchestrator-recipes
wk push
```

Four gaps produce this shape:

1. **No bulk local-subdir discovery on init.** The existing flag surface requires every `[[sync]]` entry to be enumerated explicitly. A developer whose local `./workato/recipes/` already contains four subdirectories still types four `--sync` flags.

2. **No incremental sync-entry management.** Adding or removing a single `[[sync]]` entry after init requires hand-editing `wk.toml`. Listing existing entries requires reading the TOML file directly. The CLI has no vocabulary for "add one more project to this scaffold" or "show me what's configured." ADR-005's Issue #29 section acknowledged this gap and deferred it to a follow-up; this ADR is that follow-up.

3. **No cache hygiene path outside push/pull.** The write-through cache from ADR-005 Decision 9 resolves and caches server folder IDs during sync operations, but there is no command that primes or validates the cache without transferring content. An adoption developer wanting populated `folder_id` values must force a push or pull; a developer suspecting drift (cached IDs pointing at folders that were renamed or deleted server-side) has no way to detect it until the next transfer fails. Cache hygiene is coupled to content transfer.

4. **Push requires pre-existing server folders.** `internal/sync/helpers.go` `resolveFolderID` errors with "folder not found" the moment any path segment is missing. The existing `wk folders create` command is never invoked from push. A developer following the demo will hit "folder not found" on first push with no hint that folders must be pre-created.

A fifth, more subtle gap: the CLI has no "name" concept distinct from "path." Every server-side identifier is written to `SyncEntry.ServerPath` and handled by a `/`-split hierarchy walker. In greenfield, where no hierarchy yet exists, this conflates two different mental models — *naming* the folders a developer wants to create and *pathing* to folders that already exist.

**Framing that drives the scope of this ADR:** adding sync entries to a project is *one operation with two entry points*. Bootstrap (`wk init`) and incremental (`wk sync add`) differ in timing, not in semantics. The flag surface, dedup logic, and conflict rules must work identically in both contexts. Treating them as separate features — one addressed now, one deferred — would create a cliff where developers learn one flag vocabulary at init time and a different one later. This ADR commits to a unified sync-entry lifecycle: declare, list, refresh, remove, push.

---

## Mental Model

Before the decisions, the mental model the CLI is committing to. A developer who internalizes this model should be able to predict exactly what `wk init` does without reading further documentation.

**When you run `wk init --name <container> [...]` from directory `X`:**

```
X/
└── <container>/              ← the wk project, named by --name
    ├── .wk/                  ← CLI state (gitignored per ADR-005)
    │   └── wk.toml
    ├── <project-a>/          ← each declared Workato project,
    ├── <project-b>/              one level inside the container
    └── <project-c>/
```

Three invariants:

1. **The container is always named by `--name`** and is always created inside the directory you run `wk init` from. ADR-005 Decision 1, unchanged.
2. **`.wk/` always lives at the container root.** ADR-005 Decision 5, unchanged.
3. **Workato projects live one level inside the container by default.** Each declared project is a directory at `<container>/<project>/`.

For monorepos where Workato assets are segregated from non-Workato code (the dewy-resort shape — `app/`, `tools/`, `projects/` alongside a dedicated `workato/` subfolder), invariant 3 is overridden by `--projects-dir <relpath>`:

```
Github/
└── dewy-resort/              ← --name dewy-resort
    ├── .wk/
    ├── app/                  ← non-Workato
    ├── tools/                ← non-Workato
    └── workato/
        └── recipes/          ← --projects-dir ./workato/recipes
            ├── atomic-salesforce-recipes/
            ├── atomic-stripe-recipes/
            ├── home-assistant/
            └── orchestrator-recipes/
```

**In both cases the mental model is the same:** you name the container with `--name`, and Workato projects live at a predictable path inside it. The only variable is whether that path is the container root (default) or a subfolder of the container (`--projects-dir`). Everything the CLI writes stays inside the container — the developer never has to reason about tool state leaking outside `<name>/`.

---

## Decision

Introduce a unified sync-entry flag surface (`--project`, `--projects-dir`, and a repositioned `--sync SERVER:LOCAL`) usable at both `wk init` and a new `wk sync add` subcommand. Add `wk sync list`, `wk sync refresh`, and `wk sync remove` to complete the lifecycle. Extend `wk push` to create missing top-level server folders on first sync using a resolve-then-create sequence. Remove the legacy `--server-path` / `--local-path` single-entry flags, which are subsumed by `--sync SERVER:LOCAL` and add no expressive capability.

**Greenfield first-init flow (first developer, nothing exists server-side):**

```
wk init --name dewy-resort --profile dev --projects-dir ./workato/recipes
wk push
```

**Adoption flow (team member joining, `.wk/` wipe recovery, predecessor-CLI migration):**

```
wk init --name dewy-resort --profile dev --projects-dir ./workato/recipes --verify
# done — wk.toml populated with entries AND folder_id cached in one command
```

**Majority-case greenfield (projects at container root):**

```
wk init --name my-project --profile dev --project foo --project bar
wk push
```

**Ongoing lifecycle (mid-session, whenever):**

```
wk sync add --project new-project     # add an entry
wk sync list                          # see entries and their cache state
wk sync refresh                       # hygiene check + update cache + detect drift
wk sync refresh --prune               # also remove entries whose server folders are gone
wk sync remove old-project            # explicitly remove a specific entry
```

---

## Key Design Decisions

### Part A — Declaring Sync Entries

These decisions define the flag surface used identically by `wk init` and `wk sync add`.

#### Decision 1: `--projects-dir <relpath>` — Where Workato Projects Live

**Decision:** `--projects-dir <relpath>` tells the CLI where inside the container Workato projects live. Default: `.` (the container root itself).

**Two modes, inferred from the presence of `--project` flags:**

| Flags passed | Mode | Behavior |
|---|---|---|
| `--projects-dir` alone (no `--project` flags) | Discovery | Walk `--projects-dir` one level deep; create one entry per non-hidden subdirectory. `server_path = <basename>`, `local_path = <projects-dir>/<basename>`. |
| `--projects-dir` + one or more `--project` flags | Prefix-only | No filesystem discovery. `--projects-dir` is the local path parent for each `--project` value. |
| Neither flag | No-op for this decision | Other flag shapes (`--sync`) still contribute entries. Discovery does not implicitly run. |

**Why one flag, mode-inferred:** both discovery and prefix-only answer the same structural question — *where do the project folders live locally?* — and splitting them across two flags (a prior draft had `--sync-dir` for discovery and `--recipes-dir` for prefix) creates surface area without a corresponding mental-model split. The developer's intent is "this is the parent directory for my projects"; the CLI decides whether to read names off disk or off `--project` flags based on whether the developer supplied names.

**Why default `.`:** it captures the majority case (projects at the container root) in zero flags. A developer who runs `wk init --name x --project foo --project bar` from a clean `~/work/` gets `~/work/x/foo/` and `~/work/x/bar/` — matching the mental model with no `--projects-dir` specified. The flag earns its keep only in the monorepo override case.

**Sub-rules for discovery mode:**

- **Hidden folders skipped.** Entries whose basename starts with `.` are excluded. No opt-in flag — hidden folders are almost never Workato asset containers, and adding a flag for an unrequested edge case is surface-area cost.
- **Empty result errors.** If discovery finds zero non-hidden subdirectories, the command errors with a message naming the directory. Silent zero-entry config is a footgun.
- **One level only.** No recursion. Workato "projects" are top-level folders under the workspace root; nested recipe folders belong inside a project, not alongside it.

**Implementation:** `os.ReadDir(dir)` under discovery mode, filter to entries where `IsDir()` and `!strings.HasPrefix(Name(), ".")`. Under prefix-only mode, no I/O — `--projects-dir` is just a string prepended to each `--project` value.

#### Decision 2: `--project <name>` — Name-Based Declaration

**Decision:** `--project <name>` is a repeatable flag that creates one `[[sync]]` entry per value. For each `--project foo`:

- `server_path = foo` (the name — no slashes, no colons)
- `local_path = <projects-dir>/foo`, where `<projects-dir>` is the value from Decision 1 (default `.`)
- `folder_id` omitted (populated on first push/pull per ADR-005 Decision 9, or by `wk sync refresh` per Decision 11)

**Why a name-shaped flag:** the CLI today has no "name" concept distinct from "path" — every server-side identifier is a `server_path` that gets `/`-split by the hierarchy walker. In greenfield, where no hierarchy exists yet, developers have names (what they want to create), not paths (references into existing structure). `--project` gives names first-class representation without forcing developers to pretend their names are degenerate single-segment paths in a colon-delimited string.

**On the naming collision with `--name`:** both flags use the word "project" in different domains.

- `--name` = **wk-project**: the local scaffold container — the directory that holds `.wk/wk.toml`, the value of `Name` in the config, the thing the developer is working inside.
- `--project` = **Workato project**: a top-level server-side folder. The Workato UI itself calls these "projects."

The two flags operate at different layers (local scaffolding vs. server-side entity) and collide only lexically. Both flag names are the right fit for their domain.

**On shortform:** `--project` stays longform-only. `-p` is bound to `--profile` as a persistent root flag at root.go:78 and propagates to every subcommand; a local `-p` on `--project` would silently shadow the inherited `--profile` binding — the same flag-shadowing footgun the code already has comments warning about. Capital `-P` is technically free but mixing `-p`/`-P` for related concepts is a typo trap.

**Implementation:** new flag `--project` (string array). Iterates values, produces `SyncEntry{ServerPath: name, LocalPath: filepath.Join(projectsDir, name)}` per entry.

#### Decision 3: `--sync SERVER:LOCAL` — Fine-Grained Control

**Decision:** Retain `--sync SERVER:LOCAL` (repeatable) as the fine-grained entry-declaration flag. It creates one entry per flag with both sides fully explicit, providing the surface for cases that do not fit the bulk- or name-based patterns above.

**When `--sync` is the right tool:**

- **Nested server paths.** A pre-existing Workato hierarchy like `All projects/Clients/Acme/Production` maps in one flag: `--sync "All projects/Clients/Acme/Production:./acme-prod"`. `--project` accepts only bare names (Decision 2); nested paths have no name-shaped equivalent.
- **Custom local paths that break the `<projects-dir>/<name>` convention.** When a developer wants a specific Workato project's content at `./src/workato-automations` instead of `./workato-automations`, `--sync "workato-automations:./src/workato-automations"` expresses that in one line.
- **Surgical additions alongside bulk discovery.** `--projects-dir ./workato/recipes` (discovery) can coexist with `--sync "shared-library:./shared"` in the same invocation — discovery handles the 1:1-shaped cases, `--sync` picks up the odd one that lives elsewhere on the server or in the local tree.

**Why retained and not removed:** `--sync` is fine-grained control over a single mapping, regardless of where on the server the target lives or where the developer wants it locally. Removing it would force hand-editing `wk.toml` for every case that doesn't fit the bulk/name-based patterns — a strictly worse outcome than keeping a narrow-but-expressively-complete escape hatch. The flag is not vestigial; it occupies a distinct role alongside `--project` (name-based bulk) and `--projects-dir` (discovery-based bulk).

**Positioning in the primary flow:** `--sync` is not the first flag a developer reaches for. The decision order a developer walks through is:

1. *Can I name the projects and let them live at the default location?* → `--project`
2. *Are they already on disk under a monorepo-style subfolder?* → `--projects-dir` (discovery)
3. *Is there a specific mapping that doesn't fit either pattern?* → `--sync`

Help text and docs should present the flags in that order so the mental model matches the decision flow.

**Implementation:** unchanged from today — `parseSyncFlag` (project.go:225) already handles `SERVER:LOCAL` splitting. The flag itself stays as-is; only its documentation and positioning change.

#### Decision 4: Remove Legacy `--server-path` / `--local-path`

**Decision:** Remove `--server-path` and `--local-path` from `wk init`. Their capability is fully subsumed by `--sync SERVER:LOCAL` (which handles N entries with explicit local paths). Retaining them adds a fourth entry-producing flag shape to init without expressive gain.

**Why now:** the CLI is pre-beta with no within-CLI backward-compat guarantees. Pruning the legacy single-entry shorthand while the rest of the init flag surface is being reshaped is cheaper than pruning it later as a separate change. The shortforms `-s` and `-l` become available for future use (deliberately left unassigned by this ADR — a flag shortform earns its slot by being requested, not by being available).

**Migration for anyone using the old flags:**

```
# Before
wk init --name x --profile p --server-path "Recipes/Prod" --local-path ./prod

# After
wk init --name x --profile p --sync "Recipes/Prod:./prod"
```

One-line mechanical substitution. The `--local-path` default of `.` is preserved in `--sync`'s behavior (omit the `:./path` suffix).

**Implementation:** delete `flagServerPath` and `flagLocalPath` variables from init.go, remove the flag registrations at init.go:375-376, remove the entry-assembly branch at init.go:301-312 that handled the single-entry shorthand.

#### Decision 5: Dedup and Conflict Rules Across Flag Shapes

**Decision:** `--projects-dir` (discovery), `--project`, and `--sync` all feed the same `requested []SyncEntry` slice in both `wk init` and `wk sync add`. Three rules apply:

1. **Same `(server_path, local_path)` tuple → dedup to one entry.** Existing behavior at init.go:333, preserved.
2. **Same `server_path` with different `local_path` within one invocation → hard error.** Example: `--project foo` + `--sync "foo:./custom/foo"` produces two entries with the same server path and different local paths. The command errors with a message naming both local paths. (Under mode-inferred `--projects-dir`, the discovery-vs-`--project` conflict can't occur because `--project` presence disables discovery; but cross-flag conflicts like the `--project` + `--sync` case above still need this rule.)
3. **Existing entries in `wk.toml` (preserved via `--overwrite` at init, or already-present at `wk sync add`) participate in dedup but not in the conflict check.** Hand-edited existing state is not this ADR's authority.

**Why:** Rule 2 catches flag-interaction mistakes early, before they land in `wk.toml`. Rule 3 respects the boundary that `--overwrite` established in ADR-005 (Issue #29 fix).

#### Decision 6: Local Path Traversal Guard

**Decision:** Reject `local_path` values that would escape the project container. Applies uniformly to paths produced by `--projects-dir`, `--project`-derived `local_path` values, and the local half of `--sync SERVER:LOCAL`, in both `wk init` and `wk sync add`.

**Validation rules** applied to every `local_path` candidate (including `--projects-dir` itself and each final `SyncEntry.LocalPath`):

1. **Reject null bytes.** Classic path-handling footgun; mirrors the existing guard in `validateProjectName` at init.go:411.
2. **Reject absolute paths.** `local_path` is always relative to the project root (the container that holds `.wk/`). An absolute value is either a mistake or an escape attempt.
3. **Reject traversal escapes.** After `filepath.Clean`, reject any cleaned form that begins with `..`, or that, when joined to the project root and resolved, would sit outside the project root. Catches `"../evil"`, `"./foo/../../evil"`, and similar constructions regardless of how they were assembled across flags.
4. **Reject empty strings.** Default to `.` (project root) rather than silently accepting empty input.

**Why:** Without this guard, a value like `--projects-dir "../../etc"` produces `[[sync]]` entries whose `local_path` resolves outside the container. On pull, ADR-002 Decision 1's RLCM zip extraction would write extracted files to those locations — arbitrary file overwrite. On push, the CLI would include files from locations the developer did not intend to publish to the workspace. The security boundary the CLI depends on is *everything this project manages lives inside the container*; any flag that accepts a path has to uphold that boundary. This decision is the `--projects-dir` / `--sync`-local analogue of the belt-and-suspenders check at init.go:190-193, which already enforces the same invariant for `--name`.

**Symlinks are out of scope.** The guard operates on the declared path string, not on where it resolves to at push/pull time. A symlink inside the container that points outside is a separate trust-boundary question (same posture as Git, which trusts the repo contents). If symlink-aware enforcement becomes necessary, it belongs in a future ADR scoped to filesystem trust boundaries — not this one.

**Implementation:** new `validateLocalPath(projectRoot, localPath string) error` helper in `internal/config/`. Called during entry assembly for each candidate `local_path`, and at flag-parse time for the raw `--projects-dir` value before it is joined with any `--project` name. Error messages identify which flag produced the offending value so the developer can locate the problem quickly.

#### Decision 7: `--verify` Available Across All Flag Shapes

**Decision:** `--verify` works uniformly with `--project`, discovery-mode `--projects-dir`, and `--sync SERVER:LOCAL`, in both `wk init` and `wk sync add`. When passed, the command walks the Workato folder hierarchy for every declared `server_path`, fails with a per-entry list of unresolvable paths before writing `wk.toml`, AND writes each resolved `folder_id` into the entry via the ADR-005 Decision 9 write-through cache. All entries must resolve for the command to succeed. On success, `wk.toml` lands with every entry's cache already populated — no subsequent `wk sync refresh` needed. Adoption is complete in one command.

**Why:** The predominant state developers hit is *adoption*, not greenfield. Because `.wk/` is gitignored (ADR-005 Decision 8), `wk.toml` is per-developer machine-local — so every team member who clones the repo, every re-hydrate after a `.wk/` wipe, and every predecessor-CLI migration is a `wk init` against server folders that already exist (created by the first developer's push). Greenfield (first developer, first time) is the minority case. `--verify` is genuinely useful in the adoption workflow as a precommit check that catches "my local structure doesn't match the server" *before* `wk.toml` lands on disk.

An earlier revision of this ADR made `--verify` mutually exclusive with `--project` and discovery-mode `--projects-dir` under the assumption those flags always expressed greenfield intent. That assumption was wrong for the dominant workflow — adoption inits are the common case, not greenfield — and the mutex has been lifted. Developers select their mode by passing or omitting `--verify`, not by which entry-declaration flag they chose.

**Behavior:**

- **With `--verify`** (adoption intent): init walks each declared `server_path`. Any entries that don't resolve are reported per-entry; command exits non-zero and does not write `wk.toml` if any entry fails. If all entries resolve, `wk.toml` is written with every `folder_id` populated from the walk — one command, fully adopted state.
- **Without `--verify`** (greenfield intent, or "trust me"): init writes `wk.toml` regardless of server state, `folder_id` values omitted. Push creates missing folders per Decision 12 and populates the cache on first transfer.

**Greenfield developers simply omit `--verify`.** There is no flag interaction to learn. A CI script that defensively passes `--verify` on a greenfield first init will fail — which is correct behavior: passing verify for paths you are about to create is a contradiction, and failing loudly surfaces the logic error rather than silently degrading.

**Relationship to `wk sync refresh`:** `--verify` is the *precommit* form of the same underlying walk. It validates + caches all-or-nothing before `wk.toml` is written. `wk sync refresh` (Decision 11) is the *postcommit* form — same walk and cache logic applied to entries already in `wk.toml`, partial-ok, plus drift detection for cached IDs that no longer resolve. Verify runs once at declaration; refresh runs whenever a developer wants a hygiene check. They share the underlying implementation (hierarchy walk + cache write-through) but differ in lifecycle role.

**Implementation:** extend `verifyServerPath` at init.go:38 to capture each resolved folder ID during the walk, then populate `cfg.Sync[i].FolderID` before `config.Save` writes wk.toml. Reuses the write-through cache mechanism from ADR-005 Decision 9. The flag-parse mutex check from the prior ADR revision is not implemented; the same verification + cache path runs regardless of which entry-producing flag populated the candidates.

### Part B — Sync Entry Lifecycle Commands

These decisions introduce the subcommand tree that makes incremental entry management first-class.

**Namespace rationalization.** The existing codebase places sync-entry management under `wk project sync add/list/remove` (project.go:24). This ADR promotes that tree to the top-level `wk sync add/list/refresh/remove`. The decisions below are specified under their final `wk sync X` names. Rationale:

1. **Agent-intuitive design (core CLI principle).** An agent reaching for "add a sync entry" will guess `wk sync add` before `wk project sync add`. The resource-at-top-level pattern is overwhelmingly dominant in CLIs agents have seen in training data: `git remote add` (not `git repo remote add`), `gh issue create` (not `gh repo issue create`), `kubectl delete pod` (not `kubectl cluster pod delete`), `docker volume create`. In each case the container (repo, cluster) is implicit context; the resource verb lives at the top level. Burying `sync` under `project` forces the agent to acquire domain-specific knowledge before they can form the right command — which is the kind of friction the CLI's core design principle exists to eliminate.

2. **Internal consistency with existing transfer verbs.** `wk push` and `wk pull` — the transfer verbs for the sync domain — already live at the top level. Placing the *config* verbs for the same domain (`add`, `list`, `refresh`, `remove`) one level deeper under `wk project sync` creates an arbitrary split. Developers and agents reading `wk push` will expect `wk sync list`, not `wk project sync list`. Promoting the config verbs resolves the inconsistency without forcing `push`/`pull` through a rename that would bury the hottest commands.

3. **`wk project` survives for genuinely project-scoped operations.** `wk project rm` stays where it is. Sync entries move out because they have their own lifecycle and naming vocabulary that doesn't benefit from living inside a "project operations" namespace. Commands that genuinely operate on the project as a unit (future `wk project rename`, `wk project archive`, etc.) remain under `wk project`.

4. **Pre-beta rename is acceptable.** The existing `wk project sync` commands have no within-CLI backward-compat obligation; pre-beta is the right window for namespace rationalization. The code footprint to move (project.go:20-33 and related subcommand functions) is small.

#### Decision 8: `wk sync add` — Incremental Entry Addition

**Decision:** `wk sync add [flags]` adds new sync entries to an existing `wk.toml` using the flag surface from Decisions 1–6.

**Behavior:**

1. Must run inside an existing wk project. Uses `FindProjectRoot` (ADR-005 Decision 5) to locate `wk.toml`; errors if not found.
2. Collects new entries from `--projects-dir` (discovery or prefix-only mode), `--project`, and `--sync`.
3. Applies Decision 5 dedup (against both the current invocation's entries and the existing `cfg.Sync`) and the Decision 5 conflict rule.
4. Honors `--verify` per Decision 7 (walks the hierarchy, fails fast on any unresolvable entry before writing wk.toml).
5. New entries start with `folder_id` omitted; populated on first push/pull or via `wk sync refresh`.
6. Writes the updated config to `wk.toml`.
7. Reports added-entry count. Zero-add is a warning (not an error) — the flags may all have dedup'd against existing entries, which is a valid no-op.
8. `--json` emits structured output via `rctx.Formatter`.

**Why a subcommand rather than reusing `wk init --overwrite`:** `--overwrite` replaces the project scaffold including non-sync fields (`Name`, `Profile`, workspace/environment/email snapshot). `wk sync add` is scoped to entry management — it leaves everything else alone. ADR-005's Issue #29 fix made `--overwrite` preserve existing sync entries, so it can be used to add entries today, but the semantic mismatch (a "replace scaffold" command used for "add one entry") is what `wk sync add` fixes.

**Implementation:** new file `internal/commands/sync_add.go`, registered under the existing `wk sync` command group. Reuses the flag-binding and entry-assembly helpers extracted from `wk init` so the two commands are guaranteed to stay in sync.

#### Decision 9: `wk sync list` — Entry Enumeration

**Decision:** `wk sync list` prints all sync entries in the current project with default columns: `SERVER_PATH`, `LOCAL_PATH`, `FOLDER_ID`.

**Output modes:**

- **Default (human):** tabular output. Entries where `folder_id == 0` render as `—` in the folder ID column, signaling "not yet resolved" — making unresolved state visible without requiring a separate flag.
- **`--verbose`:** adds a last-synced timestamp column, read from the most-recent sidecar metadata in `.wk/` for files under the entry's `local_path`. Absent if the entry has never been pulled or pushed.
- **`--json`:** structured array output via `rctx.Formatter`. Includes all fields in the `SyncEntry` struct plus the derived last-synced timestamp.

**Why this shape:** list is a read-only command run frequently (before sync add, before push, as a "what's in this project" sanity check). Default output must be fast and legible; verbose output must not require an API call (metadata-only, same principle as `wk status` in ADR-002 Decision 5); JSON output must be consumable by wrapping scripts.

**Relationship to `wk sync refresh`:** `wk sync list` shows entries as-is. If `folder_id` columns show `—`, that's a signal to run `wk sync refresh` (Decision 11) to populate the cache — the combination gives developers a "see unresolved state, then resolve it" loop without triggering a content transfer.

**Implementation:** new file `internal/commands/sync_list.go`. Reads `cfg.Sync` directly; optional metadata lookup in `--verbose` mode reuses the existing `.wk/` sidecar scanning logic.

#### Decision 10: `wk sync remove` — Entry Removal

**Decision:** `wk sync remove <server_path>` removes the sync entry with matching `server_path` from `wk.toml`.

**Behavior:**

1. Resolves `<server_path>` against `cfg.Sync`. If zero matches, error "no sync entry matches server path %q." If multiple matches (should be impossible after Decision 5 is enforced, but defensive), error naming all matches and ask the developer to disambiguate via `--local-path`.
2. Alternate disambiguator: `--local-path <path>` pairs with `<server_path>` to uniquely identify an entry when duplicates exist from pre-ADR hand-editing.
3. Removes the entry from `cfg.Sync`, writes `wk.toml`.
4. **Does NOT delete local files by default.** **Does NOT delete the server-side folder.**
5. `--purge` flag also deletes the local directory and its `.wk/` meta mirror. Parity with `wk project rm --purge` (project.go:216).
6. Interactive confirmation prompt when `--purge` is passed; non-interactive requires `--yes` to proceed.
7. `--json` emits structured output.

**Why no server-side deletion:** folder deletion is destructive across the workspace boundary. `wk folders delete` remains the explicit, separately-authorized surface for that action. Implicit server-side deletion via entry removal would violate the "executing actions with care" principle — a sync-entry removal should be a local bookkeeping operation, not a remote destructive one.

**Why `--purge` matches `project rm`:** developers who learn the project-level purge convention should not have to learn a different flag name at the sync-entry level. Consistency reduces cognitive cost.

**Implementation:** new file `internal/commands/sync_remove.go`. Reuses the confirmation-prompt helper from `wk project rm`.

#### Decision 11: `wk sync refresh` — Mid-Session Hygiene and Drift Detection

**Decision:** `wk sync refresh` is the postcommit form of the same walk `--verify` performs at declaration time. It classifies every entry in `cfg.Sync` against current server state, updates the cache in place, and optionally prunes entries whose server folders no longer exist. It runs without push, pull, or any RLCM traffic — folder hierarchy API only.

**Entry states.** For each entry in `cfg.Sync`, refresh classifies into one of four states:

| State | Condition | Cache action |
|---|---|---|
| **resolved** | `folder_id == 0` AND hierarchy walk by `server_path` succeeds | Write resolved ID into `wk.toml` |
| **current** | `folder_id != 0` AND `GET /folders/{id}` returns 200 | No-op (cache already valid) |
| **stale** | `folder_id != 0` AND `GET /folders/{id}` returns 404 | No cache change (report only; `--prune` removes entry) |
| **not-found** | `folder_id == 0` AND hierarchy walk fails | No cache change (report only; `--prune` removes entry) |

**Default behavior** (no `--prune`):

1. Walk every entry, classify into one of the four states above.
2. For `resolved` entries: write the new `folder_id` into `wk.toml`.
3. For `current` entries: no cache write needed.
4. For `stale` and `not-found` entries: report per-entry to stderr, make no changes to `wk.toml`.
5. Print a summary of counts and a hint to re-run with `--prune` if any entries need removal.
6. Exit zero even if some entries are stale or not-found — partial results are a useful outcome; hard-erroring on the first problem would force the developer to fix or hand-remove it before any other entries could be refreshed.

**`--prune` behavior:**

1. Same classification pass.
2. After reporting, remove `stale` and `not-found` entries from `cfg.Sync` and write `wk.toml`.
3. Interactive mode: prompt for confirmation showing which entries will be removed. Non-interactive: requires `--yes` to proceed.
4. `--prune` does NOT delete local files. Local cleanup is a separate explicit decision handled by `wk sync remove --purge`. Keeping these two destructive operations on separate commands prevents "I wanted to clean up my config" from silently becoming "I lost local work."

**Output modes:**

- **Default (human):** per-entry lines with state, server path, and folder_id (or reason for stale/not-found). Summary line with counts.
- **`--json`:** structured array of `{server_path, state, folder_id, message}` objects. Script-consumable for CI drift detection.

**Example output (default mode):**

```
Refreshing 4 sync entries...
  resolved    atomic-salesforce-recipes       (folder_id=48291)
  current     atomic-stripe-recipes           (folder_id=48292)
  stale       home-assistant                  (cached folder_id=48293 no longer exists)
  not-found   orchestrator-recipes            (server_path does not resolve)

Summary: 1 resolved, 1 current, 1 stale, 1 not-found.
2 entries need attention. Run `wk sync refresh --prune` to remove stale and not-found entries.
```

**Why this shape:**

- **Four-state classification (not just resolve/not-found).** The `current` vs `stale` distinction requires validating cached IDs, not just resolving missing ones. An earlier draft skipped validation for already-cached entries; that would have made refresh useless as a drift-detection tool. The cost of validation is one `GET /folders/{id}` per cached entry — cheap enough to run on every refresh.
- **Report-by-default, prune-by-opt-in.** Removing entries from `wk.toml` is a destructive change to committed state. Making it default-on would violate the "executing actions with care" principle. Requiring `--prune` explicitly says "I saw the report and I want them gone."
- **No exit-nonzero on stale/not-found entries.** A developer running refresh wants to see the current state of every entry, not block on the first problem. Scripted uses can check the JSON output or count field for drift alarms without relying on exit codes.
- **Relationship to `--verify`.** Same walk, same cache write-through, different lifecycle role. Verify is the precommit all-or-nothing form; refresh is the postcommit partial-ok form with drift detection added. Developers learn one concept (walk + cache) applied at two points.

**When developers run refresh:**

- After `wk init` without `--verify` (greenfield mode) to prime the cache before starting real work.
- Mid-session when `wk sync list` shows unexpected `—` or when push/pull starts producing "folder not found" errors.
- On a schedule or pre-push hook for scripted drift detection.
- After a teammate reorganized server-side folders via the Workato UI.
- After a predecessor-CLI migration to reconcile hand-edited entries against current server state.

**Implementation:** new file `internal/commands/sync_refresh.go`. Loads `cfg` via `FindProjectRoot`, iterates `cfg.Sync`, applies the four-state classification via the existing `resolveFolderID` (for `folder_id == 0` entries) and a new lightweight `folders.Get(id)` call (for `folder_id != 0` entries — a single endpoint per entry, not the multi-step hierarchy walk). Writes cache updates via `config.Save` with the existing write-through pattern. `--prune` reuses the confirmation-prompt helper from `wk sync remove`. Structured per-entry results surfaced through `rctx.Formatter`.

### Part C — Push Semantics

These decisions extend `wk push` to close the greenfield gap between init/sync-add and a working first push.

#### Decision 12: Resolve-Then-Create on First Push

**Decision:** `internal/sync/helpers.go` `folderIDForEntry` gains a create branch. The new lookup sequence is:

1. If `entry.FolderID != 0`, use the cached ID directly. Unchanged from ADR-005 Decision 9.
2. Otherwise, walk the hierarchy via `resolveFolderID` by `server_path`.
3. If `resolveFolderID` returns a "folder not found" error AND `server_path` is a bare name (no `/`) AND `--no-create` was not passed, call the existing `folders.Create` API, cache the returned ID via write-through to `wk.toml`, and retry the RLCM import for that entry.
4. All other error paths (nested not-found without `--create-path`, API errors, auth errors) surface unchanged.

**Why:** This closes the silent gap between init/sync-add and push. Three justifications motivate resolve-first-then-create (rather than create-always):

1. **Idempotency of a crashed push.** If the first push creates the folder but crashes before writing `folder_id` back to `wk.toml`, a retry with create-always would produce a second folder with the same name. Resolve-first finds the existing folder from the crashed attempt.
2. **Predecessor-CLI migration.** Developers moving from the old Workato CLI may already have target folders server-side. First push should adopt them, not duplicate them.
3. **Re-init after wiping `.wk/`.** `.wk/` is gitignored (ADR-005 Decision 8). Accidental `rm -rf`, branch switches that lose untracked state, and template re-clones all produce the "no cached ID, folder exists server-side" state. Do not duplicate.

The implementation cost of resolve-then-create versus create-always is one additional branch — the existing `resolveFolderID` call is reused, not added — so robustness for the three cases above is essentially free.

**Composition with `wk sync refresh`:** a developer who runs `wk sync refresh` after declaration already has `folder_id` populated before push, so push skips the resolve branch entirely and goes straight to RLCM import. Refresh and the push create-branch compose: refresh handles declaration-to-cache, push handles cache-to-transport.

**Composition with ADR-005 Decision 9:** the existing folder-ID cache invalidation path already does "resolve → cache → retry" when a cached ID returns 404 from the server. This ADR adds one more branch to the same helper: "resolve failed with not-found → create → cache → import."

**Implementation:** `internal/sync/helpers.go` — augment `folderIDForEntry` with the create branch. New unexported helper wraps `folders.Create` + cache write-through.

#### Decision 13: Gating for Create-on-Push

**Decision:** Auto-create behavior is gated as follows:

| `server_path` shape | Default | Opt-out | Opt-in |
|---|---|---|---|
| Bare name (no `/`) | Create | `--no-create` | n/a |
| Nested path (contains `/`) | Error as today | n/a | `--create-path` |

**Why:** A bare name is the signal for "I am declaring a top-level Workato project" — the greenfield pattern. A nested path is the signal for "I am pathing into an existing hierarchy" — if that hierarchy is missing, the likelier explanation is a typo than an intent to auto-create a multi-level tree.

- `--no-create` is the CI escape hatch. In production deploys, silent folder creation is a correctness risk; an explicit opt-out lets ops gate on it.
- `--create-path` is the "I really do want to create `Foo/Bar/Baz` from scratch" escape hatch. Rare, but possible in a deeply-organized workspace. Supported for completeness.

**Trade-off made explicit:** auto-create on bare names means a typo like `--project atomic-salesforce-recipees` creates a garbage folder rather than failing loudly. Three mitigations:

1. `--verify` remains available across all entry-producing flags (Decision 7). Developers who want typo protection at declaration time opt in; the flag is compatible with `--project`, `--projects-dir`, and `--sync` alike.
2. Push output loudly reports every folder creation (Decision 14), so the typo is visible on the first push.
3. `wk folders delete` lets a developer clean up a garbage folder once they spot the typo.

This trade is acceptable for the greenfield onboarding win but is the behavior reviewers are most likely to push on.

**Implementation:** `internal/commands/sync.go` — add `--no-create` and `--create-path` flags to `wk push`. `internal/sync/push.go` — thread the two flags into `folderIDForEntry`.

#### Decision 14: First-Push Folder Creation Output

**Decision:** When push creates one or more server folders during a run, it reports each creation to stderr in human output mode:

```
Created server folder "atomic-salesforce-recipes" (folder_id=48291)
Created server folder "atomic-stripe-recipes" (folder_id=48292)
Created server folder "home-assistant" (folder_id=48293)
Created server folder "orchestrator-recipes" (folder_id=48294)
Importing packages...
```

Under `--json`, creation events are included as a structured array in the push result:

```json
{
  "folders_created": [
    {"server_path": "atomic-salesforce-recipes", "folder_id": 48291},
    {"server_path": "atomic-stripe-recipes", "folder_id": 48292},
    {"server_path": "home-assistant", "folder_id": 48293},
    {"server_path": "orchestrator-recipes", "folder_id": 48294}
  ],
  "import_results": [...]
}
```

**Why:** Folder creation is a side effect the developer did not explicitly request in the push invocation. Silent creation hides the typo-garbage-folder case (Decision 13 trade-off) and makes the first-push state illegible in terminal and CI logs. Loud, structured reporting makes the behavior auditable both interactively and in scripts.

**Implementation:** `internal/sync/push.go` — accumulate `FolderCreated` records during `folderIDForEntry` calls, surface on the push result struct. `internal/commands/sync.go` — format via the existing `rctx.Formatter` registry.

---

## Impact on Existing Commands

| Command | Change Required | Details |
|---|---|---|
| `wk init` | **Yes** | Add `--project`, `--projects-dir`. Reposition `--sync` as fine-grained control (no behavior change, documentation only). Allow `--verify` uniformly across all entry-producing flags (Decision 7 — no mutex). Extend dedup with same-server-path-different-local-path conflict rule. Enforce local-path traversal guard (Decision 6). **Remove `--server-path` and `--local-path`.** |
| `wk sync add` | **Moved + extended** | Promoted from `wk project sync add`. Replaces positional `<server-path> [local-path]` signature with the unified flag surface (`--project`, `--projects-dir`, `--sync`). `--verify` now caches `folder_id`. |
| `wk sync list` | **Moved + extended** | Promoted from `wk project sync list`. Adds `--verbose` (last-synced timestamp column) and `--json` output. Unresolved entries render `folder_id` as `—`. |
| `wk sync refresh` | **New** | Mid-session hygiene command with no predecessor under `wk project sync`. Classifies every entry against current server state (resolved / current / stale / not-found), writes cache updates, reports drift. `--prune` removes entries whose server folders are gone. No push/pull traffic. |
| `wk sync remove` | **Moved + extended** | Promoted from `wk project sync remove`. Adds `--purge` for local directory + `.wk/` meta mirror cleanup. Never touches server state. |
| `wk project` | **Yes** | `sync` subcommand group removed and promoted to top-level `wk sync`. `wk project rm` and any future project-scoped commands remain. |
| `wk push` | **Yes** | Add `--no-create`, `--create-path` flags. Wire resolve-then-create branch into `folderIDForEntry`. Report created folders on stderr and in JSON. |
| `wk pull` | **No** | Pull implies existing server state; no create branch needed. Cached-ID invalidation from ADR-005 Decision 9 still applies unchanged. |
| `wk clone` | **No** | Single-entry today. Bulk discovery and name-based flags not in scope for clone (see "What We'll Need to Revisit"). |
| `wk folders create` | **No** | Retained as the direct user-facing surface for folder creation outside push. Push reuses the same underlying API. |

---

## Consequences

### What Becomes Easier

- **Two-command greenfield flow.** `wk init --name x --projects-dir ./workato/recipes` followed by `wk push` scaffolds a multi-project demo repo like dewy-resort end-to-end with no manual folder management. Nine commands to two.
- **One-command majority case.** `wk init --name x --project foo --project bar` covers the default "projects at the container root" pattern with no `--projects-dir` needed.
- **First-class incremental entry management.** Adding or removing a single project after init is `wk sync add --project foo` or `wk sync remove foo` — no wk.toml hand-editing, no re-scaffolding with `--overwrite`.
- **One-command adoption.** `wk init --verify` validates every declared entry against server state AND caches the resolved `folder_id` in a single walk. Team members joining an existing project, developers recovering from a `.wk/` wipe, and predecessor-CLI migrators all get fully-populated `wk.toml` without a follow-up command.
- **Mid-session drift detection and cleanup.** `wk sync refresh` classifies every entry against current server state (resolved, current, stale, not-found) and surfaces drift without requiring push or pull. `--prune` removes entries whose server folders are gone. Developers can run it on a schedule, before push, or reactively when they suspect divergence — turning cache hygiene into a first-class, visible operation.
- **Consistent vocabulary across entry points.** The same flag surface works at init and at `wk sync add`. A developer who learned the flags once does not have to re-learn them for incremental work.
- **Three flags, three mental models, no overlap.** `--project` for name-based bulk, `--projects-dir` for discovery-based bulk, `--sync` for fine-grained single mappings. Each occupies a distinct role; the developer picks by answering the question their situation fits.
- **Single mental model for where things live.** Container named by `--name`; Workato projects inside by default; `--projects-dir` overrides only for the monorepo case. Nothing tool-managed leaks outside the container.
- **Idempotent first push.** Resolve-then-create means a retried push after a crash does not duplicate folders. Wiping `.wk/` and re-running does not duplicate either.

### What Becomes Harder

- **Init flag surface churn.** `--server-path` and `--local-path` are removed. Any existing script or worked example that uses them must migrate to `--sync SERVER:LOCAL` (a one-line mechanical substitution documented in Decision 4). Acceptable per the "no within-CLI backward compat" rule — we have no prior-version users — but a real change that worked examples in `docs/` need to reflect.
- **Typo tolerance shifts.** Auto-create on bare names means a typo in `--project` creates a garbage folder rather than failing loudly. Mitigated by loud push output and `wk folders delete`, but a real behavior change from today's fail-loud baseline.
- **`--verify` now carries real weight across all init inputs.** In the adoption workflow (the common case per Decision 7's framing), developers who want typo protection should pass `--verify`. Defensive `--verify` in a CI script that is genuinely greenfield will fail — that is the intended signal, not a regression, since passing verify against paths you are about to create is a contradiction.
- **Push has creation semantics.** Today's push is a write-only-into-existing-folders operation. Adding create changes the command's side-effect profile; operations teams need to know about `--no-create` for production gating.
- **Mode-inferred `--projects-dir`.** The flag behaves differently based on whether `--project` is also present (discovery vs. prefix-only). This is documented in the mental model and the decision, but it is a subtlety a new developer has to learn. Accepted because the alternative (two separate flags for the same structural concept) was worse.
- **Namespace rename — `wk project sync X` → `wk sync X`.** Three existing subcommands (`add`, `list`, `remove`) move from `wk project sync` to `wk sync`, and `refresh` joins them as new. Agent-intuitive per the CLI's core design principle, but a breaking rename for any early script or tutorial that referenced the `wk project sync` form. Pre-beta is the right window for this.
- **New subcommand surface alongside the rename.** The sync command family now sits at `wk sync add/list/refresh/remove` alongside top-level `wk push`/`wk pull`/`wk status`/`wk diff`. Help text and discoverability need to cover them as one coherent group.

### What We'll Need to Revisit

- **Bulk discovery depth.** If teams organize Workato assets into deeper local hierarchies (e.g., `./recipes/<category>/<project>/`), the one-level discovery walk will not cover them. Composition with `--project` gives an escape valve in the meantime. Revisit if demand emerges.
- **Clone parity.** `wk clone` remains single-entry. If developers start wanting to clone multiple Workato projects into one local scaffold, this ADR's flag shapes become the model to port.
- **Per-entry create gating.** Decision 13 gates auto-create at the invocation level (`--no-create` affects all entries in a push). If a project has a mix of "these are greenfield" and "those are expected-to-exist," per-entry gating stored in `wk.toml` may become necessary.
- **Scheduled / pre-push hook integration for `wk sync refresh`.** The JSON output mode is designed to be script-consumable; if teams start wiring it into pre-push hooks or CI drift checks, conventions (exit codes? threshold flags?) may need formalizing. Deferred until that use case shows up.
- **Sync entry rename / move.** `wk sync remove` + `wk sync add` works, but a first-class `wk sync rename` or `wk sync mv` may be wanted if renames become common.
- **Team sharing of `wk.toml`.** Whether teams want to share a single `wk.toml` across developers is an open question. For most cases, sharing the local file/folder structure (directory layout, asset content) is probably the effective mechanism — each developer runs `wk init` fresh against their own profile, and the committed tree carries everything Workato-relevant. The more likely structural case for multiple `wk.toml` files is a customer monorepo containing an entire workspace, where per-team `wk.toml` files scope each team's slice of the repo. That pattern already works under ADR-005's sibling-project convention (each team's slice becomes its own container with its own `.wk/wk.toml`) — no new command needed. The sharing question only arises if developers end up wanting one `wk.toml` shared across multiple profiles; if that use case surfaces in practice, candidate future designs include a `wk adopt` command, a `wk init --adopt` mode, or splitting `wk.toml` into committable (entries) and machine-local (profile snapshot, folder-ID cache) halves. Deferred — no sense picking a shape before the use case is.

---

## Supersedes / References

- **ADR-005** "Related: Issue #29 — `init` sync-entry ergonomics": the "Incremental sync management (deferred)" note carved out `wk sync add/list/remove` plus bulk scaffolding as follow-up work. **This ADR closes both halves** and adds `wk sync refresh` on top. No part of the Issue #29 deferral remains open.
- **ADR-002 Decision 4** (folder resolution by hierarchy walk): this ADR adds a post-walk create branch when the walk errors "not found" and gating rules permit (Decision 12), and a standalone maintenance command that exercises the same walk (Decision 11). The walk itself is unchanged.
- **ADR-005 Decision 9** (folder-ID caching in `wk.toml`): the new create branch and `wk sync refresh` both use the same write-through cache mechanism.
- **ADR-006** (profile identity model): unchanged.

---

## Post-Implementation Notes

These record divergences from the original spec that came up during implementation. The Decision text above is preserved as the design intent; the notes below are the as-built reality.

- **Decision 5 rule 1 — exact duplicates are errors, not silent dedup.** The original rule said identical `(server_path, local_path)` tuples within one invocation dedup to one entry. Implementation errors instead: a developer who typed `--project foo --project foo` almost certainly made a typo, and silent dedup hides it. The conflict-rule machinery now flags both duplicates and same-server-path-different-local-path with distinct error messages. Against existing `wk.toml` entries, exact matches still silently skip per Decision 8 rule 3.

- **Decision 7 — `--overwrite` replaces rather than preserves.** Described in the Decision text, but worth calling out: the Issue #29 preservation workaround (keeping hand-edited `[[sync]]` entries through `--overwrite`) was obsoleted by the new flag surface. `--overwrite` now replaces the wk.toml in full. For incremental edits, use `wk sync add` / `wk sync remove`.

- **Decision 11 — four states, not four-plus-API-validation.** The original spec used `GET /folders/{id}` to distinguish `current` (200) from `stale` (404) for already-cached entries. That endpoint does not exist in the Workato API. Implementation validates cached IDs by walking the hierarchy via `List` and comparing the resolved leaf ID against the cached value. The four states became:
  - `found` — no cache, walk succeeded (first-time resolve)
  - `current` — cache matches walk result
  - `repaired` — cache present but walk returned a different id; auto-healed (a case the original `Get`-based design couldn't see)
  - `not-found` — walk failed. Cached entries retain their pre-classify `folder_id` in the output so "used to work" vs "never worked" stays visible without a fifth state.

- **Decisions 7, 12, 14 — project_id alongside folder_id.** The Workato folders list returns both `is_project` and a distinct `project_id` when the folder is a top-level project. `DELETE /projects/{project_id}` requires the project id (not the folder id), so `SyncEntry` gained a `ProjectID` field stored alongside `FolderID`. Every cache-fill path (`--verify`, `wk sync refresh`, push's create branch) captures both, and `wk folders delete` routes by `IsProject`, passing `project_id` when true.

- **`--no-input` promoted to a root persistent flag.** Originally init-local, but the contract "commands that don't prompt should still accept `--no-input` as a no-op" is cleanest as a root persistent flag. Scripts can pass `--no-input` uniformly without learning which subcommands honor it.

- **Folder-list memoization during `wk sync refresh`.** N entries sharing a workspace would refetch the same folder list N times. `SyncEngine.EnableFolderListCache()` opts into per-parent memoization for the duration of a sweep; pull/push/status keep the cache disabled so they always see fresh server state.

- **Pre-existing retry-predicate fix.** `push.go`'s cache-invalidation retry originally gated on the pass-by-value `entry.FolderID`, which stayed zero when a folder was freshly resolved or created mid-push. Fixed alongside the create branch (Decision 12): the retry now checks the returned `folderID` instead.

- **`wk project` namespace deleted.** With the sync subtree moved out, nothing remained under `wk project`. The empty parent namespace was removed rather than kept as a placeholder; a future `wk project rm` / `wk project rename` can reintroduce it when there's a concrete command to register.

---

## Action Items

1. [ ] Extract shared flag-binding and entry-assembly helpers from `wk init` into `internal/commands/sync_flags.go` (or similar) so `wk sync add` can reuse them without duplication
2. [ ] Add `--project`, `--projects-dir` flags to `wk init`
3. [ ] Implement `--projects-dir` with mode inference (discovery when no `--project` flags present; prefix-only otherwise); enforce hidden-folder filter and empty-result error under discovery
4. [ ] Implement `--project` entry generation (composes with `--projects-dir` for `local_path`)
5. [ ] **Remove `--server-path` and `--local-path` flags and their entry-assembly branch from `wk init`** (init.go:375-376 and init.go:301-312)
6. [ ] Reposition `--sync` in init's help text per Decision 3 (fine-grained control; third choice after `--project` and `--projects-dir`)
7. [ ] Extend `--verify` to cache resolved `folder_id` values during the hierarchy walk (Decision 7). Runs uniformly across `--project`, `--projects-dir` (discovery and prefix-only), and `--sync` in both init and `wk sync add`; all-resolve-or-fail; no flag-interaction mutex. On success, `wk.toml` lands with every `folder_id` populated from the walk
8. [ ] Add "same `server_path`, different `local_path`" hard-error in the dedup pass (init and sync add)
9. [ ] Implement `validateLocalPath(projectRoot, localPath string) error` in `internal/config/`; reject null bytes, absolute paths, `..`-traversal, and empty strings. Wire into flag-parse validation for `--projects-dir` and per-entry validation for every candidate `local_path` in init and `wk sync add`
10. [ ] Promote `wk project sync` namespace to top-level `wk sync`. Move subcommand registration out of `internal/commands/project.go` (`newProjectSyncCmd` at project.go:24 and child commands) into a new top-level `newSyncCmd` added to root.go. `wk project` retains `rm` and any future genuinely-project-scoped commands
11. [ ] Move and extend existing `wk project sync add` to `wk sync add` (`internal/commands/sync_add.go`): replace positional `<server-path> [local-path]` signature with the Decision 1–3 flag surface (`--project`, `--projects-dir`, `--sync`); keep `--verify` behavior (now caches per Decision 7)
12. [ ] Move existing `wk project sync list` to `wk sync list` (`internal/commands/sync_list.go`): extend with `--verbose` (last-synced timestamp column) and `--json` per Decision 9
13. [ ] Implement `wk sync refresh` subcommand — `internal/commands/sync_refresh.go`. New command, no predecessor under `wk project sync`. Four-state classification (resolved / current / stale / not-found) via `resolveFolderID` for uncached entries and a new `folders.Get(id)` call for cached ones. Default reports + writes cache updates; `--prune` removes stale and not-found entries with confirmation (or `--yes`). Per-entry reporting plus `--json`
14. [ ] Move existing `wk project sync remove` to `wk sync remove` (`internal/commands/sync_remove.go`): retain behavior; add `--purge` for local directory + `.wk/` meta mirror cleanup per Decision 10
15. [ ] Add `--no-create`, `--create-path` flags to `wk push`
16. [ ] Implement resolve-then-create branch in `folderIDForEntry` with write-through cache
17. [ ] Add first-push create reporting — stderr human format and structured JSON output
18. [ ] Migrate existing tests from `internal/commands/project_test.go` that exercise `project sync add/list/remove` to cover the new `wk sync X` commands; update test invocations from `{"project", "sync", ...}` → `{"sync", ...}`
19. [ ] Integration tests: greenfield first-init flow (no `--verify`, push creates folders + caches); adoption re-init flow (`--verify` resolves, caches, and writes fully-populated wk.toml in one command); `--verify` failure when declared entries don't resolve (wk.toml not written); `wk sync add` after init; `wk sync list` output shapes including `—` for uncached entries; `wk sync refresh` four-state classification against a mixed workspace (resolved/current/stale/not-found); `wk sync refresh --prune` removes stale and not-found entries with confirmation; `wk sync remove` with and without `--purge`; crashed-retry idempotency; re-init after `.wk/` wipe with folder still present
20. [ ] Unit tests: flag-surface helpers; `--projects-dir` discovery (hidden-folder filter, empty-result error, one-level walk); `--projects-dir` prefix-only mode; `--verify` uniformly across flag shapes (pass with cache write when all resolve, fail when any don't); same-server-path conflict detection; path traversal rejection (`..`, absolute paths, null bytes, empty strings) across `--projects-dir`, `--project`-composed, and `--sync`-local paths; `wk sync remove` disambiguation; `wk sync refresh` state classification (each of resolved/current/stale/not-found); `wk sync refresh --prune` confirmation and entry removal
21. [ ] Update `wk init`, `wk push`, and new `wk sync add/list/refresh/remove` `Long` help text to reflect the mental model ("when you run this from X with --name Y, here is what appears") and the flag surface
22. [ ] Update any worked examples in `docs/` that reference `--server-path` / `--local-path` — substitute to `--sync SERVER:LOCAL`; update any that reference `wk project sync X` to `wk sync X`
