# Manual Testing Plan

This document describes how to manually verify the `wk` CLI. Tests are split into two groups: local tests that need no external services, and live tests that require a Workato API token.

## Prerequisites

```sh
make build
go build -o plugins/example/example ./plugins/example/
```

Both commands must succeed before proceeding.

---

## Running the CLI from Arbitrary Directories

During development, you'll often need to test a pre-release `wk` binary against directories that are **not** the repo itself — empty folders, existing Workato projects, nested workspace layouts, etc. This section explains how.

### Build the binary

From the repo root:

```sh
go build -o bin/wk ./cmd/wk
```

The binary at `bin/wk` is self-contained. You can copy or reference it from anywhere.

### Set up an isolated environment

The CLI stores plugin and auth state under `~/.wk/` by default. To avoid polluting your real home directory (or to test with a clean slate), set `WK_HOME`:

```sh
export WK_HOME="$(mktemp -d)/wk-home"
mkdir -p "$WK_HOME"
```

When `WK_HOME` is set, the plugin registry uses `$WK_HOME/plugins/` instead of `~/.wk/plugins/`. This means plugin install, list, and hook dispatch all operate on the isolated directory.

If you also want to isolate auth/keyring state, override `HOME` as well:

```sh
export HOME="$(mktemp -d)/fake-home"
mkdir -p "$HOME"
```

### Reference the binary by absolute path

Since the binary won't be on `$PATH`, use an absolute path or alias:

```sh
WK="$(pwd)/bin/wk"

# Or create a temporary alias for the session:
alias wk="$(pwd)/bin/wk"
```

### Test scenario: empty directory (no project)

```sh
cd "$(mktemp -d)"

$WK status
# Expected: error — "not in a wk project directory"

$WK push
# Expected: same error

$WK version
# Expected: works (no project required)

$WK plugins list
# Expected: empty list or clean output (uses WK_HOME)
```

### Test scenario: freshly initialized project

```sh
TESTDIR="$(mktemp -d)"
cd "$TESTDIR"

$WK init --name my-test --workspace dev --server-path "All projects/Test" --local-path "./recipes"
cat wk.toml
# Expected: valid wk.toml with sync entry

$WK status
# Expected: "No synced assets found." (directory exists but is empty)

$WK push --dry-run
# Expected: "No changes to push." or similar
```

### Test scenario: existing project with local files

```sh
TESTDIR="$(mktemp -d)"
cd "$TESTDIR"

# Scaffold a project manually
mkdir -p recipes
cat > wk.toml <<'TOML'
workspace = "dev"

[[sync]]
server_path = "All projects"
local_path  = "recipes"
TOML

# Add a file that looks like a new asset
echo '{"name": "my recipe"}' > recipes/my_recipe.recipe.json

$WK status
# Expected: my_recipe.recipe.json shows as "new" (no .wk-meta.json sidecar)
```

### Test scenario: plugin hooks

To test hooks without installing plugins to your real `~/.wk/`:

```sh
# Build the recipe-lint stub
(cd plugins/recipe-lint && go build -o recipe-lint .)

# Install into the isolated WK_HOME
PLUGIN_DEST="$WK_HOME/plugins/recipe-lint"
mkdir -p "$PLUGIN_DEST"
cp plugins/recipe-lint/recipe-lint "$PLUGIN_DEST/"
cp plugins/recipe-lint/plugin.toml "$PLUGIN_DEST/"

# Verify it's discovered
$WK plugins list
# Expected: recipe-lint 0.1.0

# Push should invoke the hook (passthrough stub allows it)
cd "$TESTDIR"
$WK push --dry-run
# Expected: no hook warnings (--dry-run skips hooks)

$WK push
# Expected: hook runs, passthrough allows push to proceed (will fail at API stage without auth)

$WK push --skip-hooks
# Expected: hooks skipped entirely
```

### Automated smoke tests

The `test/smoke_hooks.sh` script automates the above patterns. It builds both binaries into a temp directory, sets up `WK_HOME` and `HOME`, scaffolds test fixtures, and validates hook behavior end-to-end:

```sh
./test/smoke_hooks.sh
```

It covers: no-plugin push, passthrough plugin, `--skip-hooks`, `--dry-run`, failing plugin diagnostics, and bypass of failures with `--skip-hooks`.

### Quick reference

| Variable | Purpose | Default |
|----------|---------|---------|
| `WK_HOME` | Override `~/.wk/` for plugins | `$HOME/.wk` |
| `HOME` | Override home dir for auth/keyring | System home |

| Flag | Effect |
|------|--------|
| `--skip-hooks` | Skip all plugin pre-push hooks |
| `--dry-run` | Show what would be pushed; hooks are not invoked |

### Cleanup

Temp directories created with `mktemp -d` persist until you delete them or reboot. Clean up after a session:

```sh
# If you saved the paths:
rm -rf "$WK_HOME" "$HOME" "$TESTDIR"

# Or to be safe, find recent tmpdir artifacts:
ls -dt /tmp/tmp.* | head -5
```

---

## Group 1: Local Tests (no API token)

These tests verify project lifecycle, auth UX, plugin system, and output formatting without any network calls.

### 1.1 Version

```sh
./bin/wk version
```

Expected: `wk version <semver> (commit: <hash>, built: <date>)`

```sh
./bin/wk version --json
```

Expected: valid JSON with keys `version`, `commit`, `date`.

### 1.2 Help Tree

```sh
./bin/wk --help
```

Expected: lists all top-level commands including `auth`, `init`, `link`, `recipes`, `connections`, `pull`, `push`, `status`, `diff`, `plugins`, `version`.

```sh
./bin/wk auth --help
./bin/wk recipes --help
./bin/wk connections --help
./bin/wk plugins --help
```

Expected: each shows its subcommands with descriptions.

### 1.3 Init

Set up a temp directory for each test run:

```sh
TESTDIR=$(mktemp -d)
```

**Create a project:**

```sh
cd "$TESTDIR"
wk init --name test-project --workspace dev --server-path "All projects/Test" --local-path "./recipes"
```

Expected: prints success message. A `wk.toml` file exists with:
- `name = 'test-project'`
- `workspace = 'dev'`
- A `[[sync]]` block with `server_path` and `local_path`

**Init should fail if wk.toml already exists:**

```sh
wk init --name another --workspace prod
```

Expected: error containing "wk.toml already exists".

**Non-interactive JSON mode:**

```sh
cd "$(mktemp -d)"
wk init --name json-test --workspace staging --json
```

Expected: JSON output with keys `status`, `name`, `workspace`, `path`. No non-JSON text on stdout.

**Non-interactive JSON mode requires flags:**

```sh
cd "$(mktemp -d)"
wk init --json
```

Expected: error about `--name` being required.

### 1.4 Link

```sh
cd "$TESTDIR"
wk link --workspace staging
cat wk.toml
```

Expected: `workspace` value changed to `staging`. Output confirms the change.

**Link outside a project:**

```sh
cd "$(mktemp -d)"
wk link --workspace dev
```

Expected: error about no `wk.toml` found.

### 1.5 Status with No Synced Files

```sh
cd "$TESTDIR"
wk status
```

Expected: message like "No synced assets found." (no crash, no stack trace).

### 1.6 Auth Without Profiles

```sh
wk auth status
```

Expected: error about no active profile. Clean message, not a panic.

```sh
wk auth list
```

Expected: empty list or clean error.

### 1.7 Plugins

**Install the example plugin:**

```sh
wk plugins install ./plugins/example
```

Expected: success message with plugin name and version.

**List plugins:**

```sh
wk plugins list
```

Expected: table showing `example`, `0.1.0`, and its path.

```sh
wk plugins list --json
```

Expected: valid JSON array.

**Remove plugin:**

```sh
wk plugins remove example
wk plugins list
```

Expected: remove succeeds, list is now empty.

### 1.8 Error Messages

**Recipe commands without auth:**

```sh
wk recipes list
```

Expected: clean error about missing profile or credentials (not a Go panic or stack trace).

**Push outside a project:**

```sh
cd "$(mktemp -d)"
wk push
```

Expected: error about not being in a wk project directory.

**Bad recipe ID:**

```sh
wk recipes get notanumber
```

Expected: error about invalid recipe ID.

---

## Group 2: Live Tests (requires Workato API token)

These tests make real API calls. You need a valid Workato API token and a workspace with at least one recipe and one connection.

Set these before starting:

```sh
export TEST_TOKEN="<your-workato-api-token>"
export TEST_REGION="us"  # or eu, jp, au, sg, trial
export TEST_RECIPE_ID="<id-of-a-non-critical-recipe>"
export TEST_CONN_ID="<id-of-a-connection>"
export TEST_FOLDER_ID="<id-of-a-folder-with-recipes>"
export TEST_SERVER_PATH="<full-server-path, e.g. All projects/My Project>"
```

### 2.1 Auth Login

```sh
wk auth login --token "$TEST_TOKEN" --region "$TEST_REGION" --name test-live
```

Expected: profile saved and set as active.

### 2.2 Auth Status

```sh
wk auth status
```

Expected: shows profile name, region, `has_credentials: true`.

```sh
wk auth status --json
```

Expected: valid JSON with profile details.

### 2.3 Multiple Profiles

```sh
wk auth login --token "$TEST_TOKEN" --region "$TEST_REGION" --name second-profile
wk auth list
```

Expected: two profiles listed, `second-profile` is active.

```sh
wk auth switch test-live
wk auth list
```

Expected: `test-live` is now marked active.

### 2.4 Recipes List

```sh
wk recipes list
```

Expected: table of recipes with ID, NAME, FOLDER, RUNNING, VERSION columns.

```sh
wk recipes list --json
```

Expected: valid JSON. Pipe through `jq .` to verify.

```sh
wk recipes list --folder "$TEST_FOLDER_ID"
wk recipes list --status running
```

Expected: filtered results.

### 2.5 Recipe Get

```sh
wk recipes get "$TEST_RECIPE_ID"
wk recipes get "$TEST_RECIPE_ID" --json
```

Expected: recipe detail. JSON mode returns parseable JSON.

### 2.6 Recipe Start/Stop

Pick a non-critical recipe. This changes recipe state.

```sh
wk recipes stop "$TEST_RECIPE_ID"
wk recipes get "$TEST_RECIPE_ID" --json | jq .running
# expected: false

wk recipes start "$TEST_RECIPE_ID"
wk recipes get "$TEST_RECIPE_ID" --json | jq .running
# expected: true
```

### 2.7 Recipe Export/Import

```sh
wk recipes export "$TEST_RECIPE_ID" -o /tmp/wk-recipe-export.json
cat /tmp/wk-recipe-export.json | jq .name
```

Expected: valid JSON file with recipe data.

```sh
wk recipes import /tmp/wk-recipe-export.json --folder "$TEST_FOLDER_ID"
```

Expected: recipe imported, output shows new recipe details.

### 2.8 Connections

```sh
wk connections list
wk connections list --json
wk connections get "$TEST_CONN_ID"
wk connections test "$TEST_CONN_ID"
```

Expected: list shows table, get shows detail, test reports connected/not connected.

### 2.9 Sync Workflow

This is the highest-value end-to-end test.

```sh
SYNCDIR=$(mktemp -d)
cd "$SYNCDIR"

# Create a project pointing at a real folder
wk init --name sync-test --workspace test-live --server-path "$TEST_SERVER_PATH" --local-path "./assets"
```

**Pull:**

```sh
wk pull
ls -la ./assets/
```

Expected: recipe/connection JSON files downloaded. Each has a companion `.wk-meta.json` sidecar.

**Status after pull (no changes):**

```sh
wk status
```

Expected: all files show `unchanged`.

**Modify a file and check status:**

```sh
# Pick any .json file (not a .wk-meta.json)
echo '{"modified": true}' >> ./assets/$(ls ./assets/*.json | grep -v wk-meta | head -1)
wk status
```

Expected: modified file shows `modified` status.

**Dry-run push:**

```sh
wk push --dry-run
```

Expected: shows what would be pushed. Prints "Dry run" notice. No actual upload.

**Diff:**

```sh
wk diff
```

Expected: shows differences between local and remote with hash comparison.

### 2.10 File-Store Auth (CI/CD mode)

See [docs/ci-setup.md](./ci-setup.md) for the full non-interactive and
file-store workflows. Minimum smoke:

```sh
# Inside a project directory containing a profiles.env with a "ci" record:
wk recipes list --profile ci --store-type file --json
```

Expected: the CLI reads the token from `profiles.env`, returns a valid JSON
recipe list, and never touches the OS keychain.

### 2.11 Error Handling

**Invalid token:**

```sh
wk auth login --token "bad-token" --region us --name broken
wk recipes list --profile broken
```

Expected: API returns 401. Error message mentions unauthorized.

**Non-existent resource:**

```sh
wk recipes get 999999999
```

Expected: API 404 error, clean message.

**Non-existent profile:**

```sh
wk recipes list --profile nonexistent
```

Expected: error about profile not found.

**Invalid region:**

```sh
wk auth login --token x --region zz --name bad
```

Expected: error about invalid region.

---

## Checklist Summary

| # | Test | Group | Pass Criteria |
|---|---|---|---|
| 0.1 | Empty dir behavior | Isolated | `status`/`push` error cleanly; `version` works |
| 0.2 | Fresh init | Isolated | `wk.toml` created; status says no assets |
| 0.3 | Existing project | Isolated | New files detected as "new" in status |
| 0.4 | Plugin hooks | Isolated | `WK_HOME` isolation; passthrough, skip, dry-run all correct |
| 0.5 | `test/smoke_hooks.sh` | Isolated | All 7 automated checks pass |
| 1.1 | `wk version` | Local | Prints version string; `--json` returns valid JSON |
| 1.2 | `wk --help` tree | Local | All commands listed with descriptions |
| 1.3 | `wk init` | Local | Creates valid `wk.toml`; fails if exists; JSON mode works |
| 1.4 | `wk link` | Local | Updates workspace in `wk.toml`; fails outside project |
| 1.5 | `wk status` (empty) | Local | No crash, clean message |
| 1.6 | Auth without profiles | Local | Clean errors, no panics |
| 1.7 | Plugin lifecycle | Local | Install, list, remove all work; `--json` on list |
| 1.8 | Error messages | Local | No stack traces, clear messages |
| 2.1 | `wk auth login` | Live | Stores credential, sets active profile |
| 2.2 | `wk auth status` | Live | Shows profile info with credentials |
| 2.3 | Multiple profiles | Live | Switch works, list shows correct active marker |
| 2.4 | `wk recipes list` | Live | Table output; `--json` valid; filters work |
| 2.5 | `wk recipes get` | Live | Returns recipe detail |
| 2.6 | `wk recipes start/stop` | Live | Toggles recipe running state |
| 2.7 | `wk recipes export/import` | Live | Round-trip produces valid recipe |
| 2.8 | Connections | Live | List, get, test all return expected output |
| 2.9 | Sync workflow | Live | Pull, status, modify, status, push --dry-run, diff |
| 2.10 | File-store auth | Live | `profiles.env` + `--store-type file` works without login |
| 2.11 | Error handling | Live | Bad token, bad ID, bad profile all produce clean errors |
