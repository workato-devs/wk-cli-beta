# CI setup

Running `wk` in CI/CD pipelines (GitHub Actions, GitLab CI, Jenkins, etc.)
uses the same commands as local development, but flag resolution works
differently when stdin is not a terminal. This page covers what changes in
non-interactive mode and how to write invocations that work in both.

## Interactive vs. non-interactive mode

`wk auth login` detects non-interactive mode when **any** of the following
is true:

- stdin is not a terminal (e.g. redirected, piped, running in CI)
- `--no-input` is passed explicitly
- `--json` is passed

The resolution rules for required fields change in that mode:

| Field | Interactive default | Non-interactive behavior |
|---|---|---|
| `--token` | Prompted | Required — hard fail if missing |
| `--environment` | Prompted | Required — hard fail if missing |
| `--workspace` | Introspected from `GET /users/me` | Introspected identically |
| `--name` | Auto-computed from `<region>-<workspace-slug>-<env>` | Auto-computed identically |
| `--region` | Defaults to `us` | Defaults to `us` |

The intentional asymmetry: `wk auth login --token X` succeeds in a TTY (it
prompts for environment) but fails in a pipe (it has nowhere to ask from).
Copying a working interactive command into a CI script typically means
adding `--environment <value>`.

### Why the asymmetry?

In CI, determinism beats keystroke economy. Making a pipeline success
depend on a prompt that never fires, or on remote state that might not be
populated yet, introduces flakiness. Failing loudly on a missing required
flag is the safer default. See
[ADR-006 Sub-decision 10](./ADR-006-profile-identity-model.md#10-non-interactive-mode-behavior).

## Minimum invocation in CI

```sh
wk auth login \
  --token "$WORKATO_TOKEN" \
  --environment prod
```

The CLI will:

1. Call `GET /users/me` with the token to introspect the workspace — this
   also serves as token validation. A bad token aborts before anything is
   persisted.
2. Compute the profile name as `<region>-<workspace-slug>-prod` — region
   is always the leading component (e.g., `us-acme-corp-prod`, `eu-acme-corp-prod`).
3. Write the profile and mark it active.

Subsequent commands (e.g. `wk pull`, `wk push`) resolve credentials by
profile name as normal.

## Example: GitHub Actions — keychain flow

This uses `wk auth login` to write into the runner's OS keychain. Simple,
but creates per-run keychain state.

```yaml
steps:
  - uses: actions/checkout@v4

  - name: Authenticate
    env:
      WORKATO_TOKEN: ${{ secrets.WORKATO_TOKEN }}
    run: |
      wk auth login \
        --token "$WORKATO_TOKEN" \
        --environment prod \
        --no-input

  - name: Pull recipes
    run: wk pull
```

`--no-input` is optional here since stdin already isn't a TTY, but it's a
useful explicit marker in scripts that might run locally too.

For the file-store flow (recommended for CI — no keychain, no login step),
see the next section.

## Credential storage in CI

CI pipelines typically skip `wk auth login` entirely and generate a
`profiles.env` from the pipeline's secrets manager instead. The file is
CLI-read-only (the CLI never writes it), uses a `NAME=` record delimiter,
and lives at `<project-root>/.wk/profiles.env` — alongside `wk.toml`
inside the tool-managed directory.

### GitHub Actions example with `profiles.env`

```yaml
steps:
  - uses: actions/checkout@v4

  - name: Write .wk/profiles.env
    env:
      WORKATO_TOKEN: ${{ secrets.WORKATO_TOKEN }}
    run: |
      mkdir -p .wk
      cat <<EOF > .wk/profiles.env
      NAME=ci
      WORKSPACE=acme-corp
      ENVIRONMENT=prod
      REGION=us
      TOKEN=$WORKATO_TOKEN
      EOF

  - name: Pull recipes
    run: wk pull --profile ci --store-type file
```

`--store-type file` routes explicitly to `.wk/profiles.env` and skips
keychain lookup. `--profile ci` names the record. The committed
`.wk/wk.toml` typically carries `profile = "ci"` so the two line up. See
[`profiles.env.example`](./profiles.env.example) for the full format.

**Important:** `profiles.env` holds secrets. Never commit it. Living inside
`.wk/` means it's covered by `.wk/.gitignore` automatically (ADR-005
Decision 8). The repo-level `*.env` rule that the sample `.gitignore`
templates often include is not relied on — the self-ignore in `.wk/` is
the authoritative guard.

## See also

- [ADR-006 — Profile identity model](./ADR-006-profile-identity-model.md)
  for the full design rationale, including why `/users/me` introspection
  is always enabled and `/activity_logs` environment introspection is
  deferred.
