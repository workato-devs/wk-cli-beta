# Command Reference

Every command supports `--help` for detailed usage and flags.

```
wk
  auth
    login           Create or update an auth profile
    list            List all auth profiles
    switch          Switch active profile
    status          Show active profile and test connectivity
    delete          Delete an auth profile and its stored credential
  init              Initialize a new wk project
  link              Link the current project to an auth profile
  clone             Clone a remote folder into a new local project
  recipes (recipe)
    list            List recipes
    get             Get recipe details
    start           Start a recipe
    stop            Stop a recipe
    export          Export a recipe as JSON
    import          Import a recipe from JSON file
    update          Update an existing recipe from a JSON file
    delete          Delete a recipe
    jobs            List recipe jobs
    copy            Copy a recipe to a folder
    update-connection  Update a recipe's connection
    validate        Validate recipe files (delegates to recipe-lint plugin)
    versions        List recipe version history
      comment       Set or update a version comment
  connections (conn)
    list            List connections
    get             Get connection details
    create          Create a connection
    update          Update a connection
    delete          Delete a connection
    disconnect      Disconnect a connection
  connectors (connector)
    list            List connectors (--search to filter)
  folders (folder)
    list            List folders
    create          Create a folder
    delete          Delete a folder or project
  tags (tag)
    list            List tags
    create          Create a tag
    update          Update a tag
    delete          Delete a tag
    apply           Apply a tag to recipes or connections
    remove          Remove a tag from recipes or connections
  api
    collections (collection)
      list          List API collections
      create        Create an API collection
    endpoints (endpoint)
      list          List API endpoints
      enable        Enable an API endpoint
      disable       Disable an API endpoint
  mcp
    test            Test MCP server connectivity
    tools           List tools exposed by an MCP server
  workspace
    info            Show current workspace info
    users           List workspace members
    audit-log       View workspace audit log
  sync
    add             Add sync entries to wk.toml
    list            Show all sync entries
    refresh         Reconcile cached folder IDs against workspace
    remove          Remove a sync entry from wk.toml
  pull              Pull remote assets to local project
  push              Push local changes to remote workspace
  status            Show sync status
  diff              Show local vs. remote differences
  plugins (plugin)
    install         Install a plugin from a local directory
    list            List installed plugins
    remove          Remove an installed plugin
  version           Print version info
  completion        Generate shell completions (bash, zsh, fish, powershell)
```

The `lint` command is contributed by the recipe-lint plugin when installed
(`wk lint` is equivalent to `wk recipes validate`).

## Global flags

Available on all commands:

| Flag | Description |
|---|---|
| `--json`, `-j` | Output as JSON |
| `--verbose` | Enable debug logging |
| `--quiet`, `-q` | Suppress non-essential output |
| `--profile`, `-p` | Override active workspace profile |
| `--store-type <backend>` | Override credential store backend (`keychain`\|`file`) |
| `--no-color` | Disable color output |
| `--no-input` | Force non-interactive mode |
| `--timeout <secs>` | API timeout in seconds (default 30) |
