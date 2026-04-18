# wk

**Workato CLI** -- manage your Workato workspace from the terminal.

`wk` is a command-line tool for working with Workato recipes, connections, and
sync operations. It supports multiple workspace profiles, bidirectional sync
between local files and remote workspaces, and a plugin system for extending
functionality. Every command supports `--json` for scripting and CI/CD use.

## Install

### Build from source

Requires Go 1.23+.

```sh
git clone https://github.com/workato-devs/wk-cli-beta.git
cd wk-cli-beta
make build
```

The binary is written to `./bin/wk`. Optionally install it to your `$GOPATH/bin`:

```sh
make install
```

### Homebrew (when available)

```sh
brew install workato-devs/tap/wk
```

## Getting started

### 1. Authenticate

```sh
wk auth login
```

This creates a named workspace profile. You can add multiple profiles and switch
between them:

```sh
wk auth login              # interactive setup
wk auth list               # show all profiles
wk auth switch <profile>   # change active profile
wk auth status             # verify connectivity
```

Supported regions: `us`, `eu`, `jp`, `au`, `sg`, `il`, `cn`, `trial` (Developer Sandbox).

Credentials can be stored in the system keychain, an environment variable, a
file, or HashiCorp Vault (see `--store` flag on `wk auth login`).

Running `wk` in CI/CD? See [docs/ci-setup.md](./docs/ci-setup.md) for
non-interactive flag requirements and example pipelines.

### 2. Initialize a project

```sh
mkdir my-workspace && cd my-workspace
wk init
```

This creates a `wk.toml` file in the current directory. To link an existing
directory to a workspace instead:

```sh
wk link
```

### 3. Work with recipes

```sh
wk recipes list
wk recipes get <id>
wk recipes start <id>
wk recipes stop <id>
wk recipes export <id> -o recipe.json
wk recipes import recipe.json
```

### 4. Sync

```sh
wk pull          # pull remote assets to local project
wk push          # push local changes to remote workspace
wk status        # show sync status
wk diff          # show differences between local and remote
```

## Command reference

```
wk
  auth
    login           Create or update an auth profile
    list            List all auth profiles
    switch          Switch active profile
    status          Show active profile and test connectivity
  init              Initialize a new wk project
  link              Link directory to a Workato workspace
  recipes (recipe)
    list            List recipes
    get             Get recipe details
    start           Start a recipe
    stop            Stop a recipe
    export          Export a recipe as JSON
    import          Import a recipe from JSON file
  connections (conn)
    list            List connections
    get             Get connection details
    test            Test a connection
  pull              Pull remote assets to local project
  push              Push local changes to remote workspace
  status            Show sync status
  diff              Show local vs. remote differences
  plugins (plugin)
    install         Install a plugin from a local path
    list            List installed plugins
    remove          Remove an installed plugin
  version           Print version info
  completion        Generate shell completions
```

### Global flags

| Flag | Description |
|---|---|
| `--json` | Output as JSON |
| `--verbose` | Enable debug logging |
| `--quiet` | Suppress non-essential output |
| `--profile <name>` | Override active workspace profile |
| `--no-color` | Disable color output |
| `--timeout <secs>` | API timeout in seconds (default 30) |

## Project config

Every `wk` project is defined by a `wk.toml` file at the project root. The CLI
walks up from the current directory to find it.

```toml
name = "my-project"
description = "Production workspace recipes"
workspace = "acme-prod"
plugins = ["example"]

[mcp]
auto_delegate = true
server_url = "https://mcp.example.com"

[[sync]]
server_path = "/recipes/production"
local_path = "./recipes"
include = ["*.json"]

[[sync]]
server_path = "/connections"
local_path = "./connections"
```

| Field | Purpose |
|---|---|
| `name` | Project name |
| `description` | Optional description |
| `workspace` | Workspace identifier (matches auth profile) |
| `plugins` | List of plugins to load |
| `mcp` | MCP integration settings |
| `sync` | Array of server-path-to-local-path mappings |

Each `[[sync]]` entry maps a remote path to a local directory, with an optional
`include` glob filter.

## Plugin system

Plugins extend `wk` with additional commands via JSON-RPC. A plugin is a
directory containing a `plugin.toml` manifest and an executable entrypoint.

### Installing a plugin

```sh
wk plugins install ./plugins/example
wk plugins list
```

### Plugin manifest format

```toml
name = "example"
version = "0.1.0"
description = "Example wk plugin demonstrating the JSON-RPC protocol"
entrypoint = "./example"

[[commands]]
name = "hello"
description = "Say hello from the example plugin"
method = "example.hello"
```

The `entrypoint` is the binary that `wk` spawns. Each `[[commands]]` entry
registers a subcommand under `wk <plugin-name> <command>`, routed to the
specified JSON-RPC `method`.

To remove a plugin:

```sh
wk plugins remove example
```

## Development

### Make targets

```sh
make build     # build to ./bin/wk
make test      # run all tests
make lint      # golangci-lint
make fmt       # gofmt
make tidy      # go mod tidy
make clean     # remove ./bin/
make install   # build + copy to $GOPATH/bin
```

### Project structure

```
cmd/wk/              Entry point (main.go)
internal/
  commands/          Cobra command definitions and RunContext
  api/               Workato API client
  auth/              Credential storage, profiles, regions
  config/            wk.toml parsing and project root discovery
  sync/              Pull/push sync logic
  plugin/            Plugin loading and JSON-RPC dispatch
  output/            Text and JSON formatters
  errors/            Structured error types
plugins/             Bundled plugin examples
```

### Release

Releases are built with [GoReleaser](https://goreleaser.com/). Cross-compiled
binaries are produced for linux, darwin, and windows on amd64 and arm64.

```sh
goreleaser release --snapshot --clean
```

## License

See [LICENSE](LICENSE).
