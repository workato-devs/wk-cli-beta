# Contributing to wk-cli

## API Resource Lifecycle

When working with Workato API resources (recipes, connections, API endpoints, etc.), three files must stay in sync. Automated tests enforce these contracts — see `internal/api/types_coverage_test.go` and `internal/sync/pull_test.go`.

### Adding a new API resource type

1. **Define the struct** in `internal/api/types.go` with `json` tags for all known response fields.
2. **Register expected fields** in `TestStructFieldCoverage` (`internal/api/types_coverage_test.go`).
3. **Register table columns** in `TestStructFieldCoverage_TableColumns` — list the fields that should appear in `wk <resource> list` text output.
4. **Wire up the table** in `internal/commands/<resource>.go` — the `FormatList` call must include all registered table columns.

### Adding a field to an existing resource

1. Add the field to the struct in `internal/api/types.go`.
2. Add the json tag name to `expectedFields` in `TestStructFieldCoverage`.
3. If the field should appear in table output:
   - Add it to `requiredTableFields` in `TestStructFieldCoverage_TableColumns`.
   - Add the column to the `FormatList` call in the corresponding command file.
4. Update any mock responses in the resource's `*_test.go` file to include the new field and assert it parses correctly.

### Adding a new Workato export file extension

When `wk pull` encounters a file type it doesn't recognize, it writes `type: "unknown"` to `.wk-meta.json`, which breaks lint and type-aware features.

1. Add the extension to `knownExtensions` in `TestInferAssetType_KnownWorkatoExtensions` (`internal/sync/pull_test.go`).
2. Add a matching case to `inferAssetType()` in `internal/sync/pull.go`.

### Verification

```bash
go build ./...
go test ./internal/api/... ./internal/sync/... ./internal/commands/...
```

If any of the coverage tests fail, the error message tells you exactly what to fix and where.
