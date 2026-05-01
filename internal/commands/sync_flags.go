package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/config"
)

// SyncEntryFlags backs the unified sync-entry flag surface used by both
// `wk init` and `wk sync add` (ADR-007 Decisions 1-3). Bind once with
// BindSyncEntryFlags, then call AssembleSyncEntries to turn parsed values
// into a deduplicated []config.SyncEntry.
type SyncEntryFlags struct {
	Projects       []string
	ProjectsDir    string
	ProjectsDirSet bool // true when --projects-dir was explicitly passed
	Syncs          []string
}

// BindSyncEntryFlags registers --project, --projects-dir, and --sync on
// cmd. Order matches ADR-007 Decision 3's decision-flow order so the help
// text matches the mental model (name-based, discovery-based, fine-grained).
func BindSyncEntryFlags(cmd *cobra.Command, f *SyncEntryFlags) {
	cmd.Flags().StringArrayVar(&f.Projects, "project", nil,
		"Workato project name to declare (repeatable; local_path defaults to <projects-dir>/<name>)")
	cmd.Flags().StringVar(&f.ProjectsDir, "projects-dir", ".",
		"Parent directory for Workato projects inside the container (default: container root). "+
			"Without --project, walked one level deep to discover entries; with --project, used as the local_path prefix.")
	cmd.Flags().StringArrayVar(&f.Syncs, "sync", nil,
		"Fine-grained SERVER_PATH[:LOCAL_PATH] mapping (repeatable; use when --project/--projects-dir don't fit)")
}

// AssembleSyncEntries turns parsed flag values into a []config.SyncEntry.
// projectRoot is the container directory (may be empty at init time before
// the container is created); when set, each entry's LocalPath is validated
// against it. Returns entries in the order they were declared, deduplicated
// by (ServerPath, LocalPath) tuple, with a hard error on same-ServerPath
// different-LocalPath conflicts within the invocation (Decision 5 rule 2).
func AssembleSyncEntries(f *SyncEntryFlags, projectRoot string) ([]config.SyncEntry, error) {
	if err := config.ValidateLocalPath("", f.ProjectsDir); err != nil {
		return nil, fmt.Errorf("--projects-dir: %w", err)
	}

	var entries []config.SyncEntry

	discovery := len(f.Projects) == 0 && (f.ProjectsDir != "." || f.ProjectsDirSet)
	if discovery {
		// --projects-dir is declared relative to the container (ADR-007
		// Decision 1, mental model: "where inside the container Workato
		// projects live"), so the on-disk walk joins it onto projectRoot
		// before reading. The stored local_path stays container-relative.
		walkDir := f.ProjectsDir
		if projectRoot != "" {
			walkDir = filepath.Join(projectRoot, f.ProjectsDir)
		}
		names, err := discoverProjectDirs(walkDir, f.ProjectsDir)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			entries = append(entries, config.SyncEntry{
				ServerPath: name,
				LocalPath:  filepath.Join(f.ProjectsDir, name),
			})
		}
	}

	for _, name := range f.Projects {
		if strings.ContainsAny(name, "/\\:") {
			return nil, fmt.Errorf("--project %q must be a bare name (no slashes or colons); use --sync for nested paths", name)
		}
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("--project value cannot be empty")
		}
		entries = append(entries, config.SyncEntry{
			ServerPath: name,
			LocalPath:  filepath.Join(f.ProjectsDir, name),
		})
	}

	for _, raw := range f.Syncs {
		e, err := parseSyncFlag(raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	for i := range entries {
		if err := config.ValidateLocalPath(projectRoot, entries[i].LocalPath); err != nil {
			return nil, err
		}
	}

	return conflictCheck(entries)
}

// discoverProjectDirs walks walkDir one level deep and returns the names of
// non-hidden subdirectories. Errors if walkDir cannot be read or contains no
// matching subdirectories (Decision 1 sub-rules). displayDir is the value
// the developer typed on --projects-dir, used in error messages so the
// output references the flag input rather than the resolved absolute path.
func discoverProjectDirs(walkDir, displayDir string) ([]string, error) {
	ents, err := os.ReadDir(walkDir)
	if err != nil {
		return nil, fmt.Errorf("--projects-dir %q: %w", displayDir, err)
	}
	var names []string
	for _, ent := range ents {
		if !ent.IsDir() {
			continue
		}
		if strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		names = append(names, ent.Name())
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("--projects-dir %q contains no non-hidden subdirectories to declare as projects", displayDir)
	}
	return names, nil
}

// conflictCheck errors on any same-ServerPath collision within the
// invocation — both the exact-duplicate case (Decision 5, `--project foo
// --project foo`: typed twice, likely a typo) and the conflicting-local-
// path case (Decision 5 rule 2). Silently deduping identical tuples hides
// typos and paste errors in a way that's hard to debug later, so both
// shapes surface as explicit errors with distinct messages. Existing
// on-disk entries from wk.toml are not considered here.
func conflictCheck(entries []config.SyncEntry) ([]config.SyncEntry, error) {
	seen := make(map[string]config.SyncEntry, len(entries))
	for _, e := range entries {
		if prior, ok := seen[e.ServerPath]; ok {
			if prior.LocalPath == e.LocalPath {
				return nil, fmt.Errorf("server path %q declared twice with the same local path %q — remove the duplicate flag",
					e.ServerPath, e.LocalPath)
			}
			return nil, fmt.Errorf("server path %q declared with conflicting local paths: %q and %q",
				e.ServerPath, prior.LocalPath, e.LocalPath)
		}
		seen[e.ServerPath] = e
	}
	return entries, nil
}

// parseSyncFlag parses a --sync value of the form "SERVER_PATH" or
// "SERVER_PATH:LOCAL_PATH" into a SyncEntry. Server paths are Workato
// forward-slash paths and never contain colons, so the first colon is the
// separator. Local path defaults to defaultLocalPathForServerPath when
// omitted.
func parseSyncFlag(raw string) (config.SyncEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return config.SyncEntry{}, fmt.Errorf("--sync value cannot be empty")
	}
	var server, local string
	if i := strings.IndexByte(raw, ':'); i >= 0 {
		server = strings.TrimSpace(raw[:i])
		local = strings.TrimSpace(raw[i+1:])
	} else {
		server = raw
	}
	if server == "" {
		return config.SyncEntry{}, fmt.Errorf("--sync %q missing server_path", raw)
	}
	if local == "" {
		local = defaultLocalPathForServerPath(server)
	}
	return config.SyncEntry{ServerPath: server, LocalPath: local}, nil
}

// defaultLocalPathForServerPath picks a sensible local dir for a server
// path when none was provided: the last /-separated segment, prefixed with
// "./", with "All projects" stripped if present.
func defaultLocalPathForServerPath(serverPath string) string {
	trimmed := strings.Trim(serverPath, "/")
	if trimmed == "" {
		return "."
	}
	parts := strings.Split(trimmed, "/")
	if strings.EqualFold(parts[0], "All projects") {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "."
	}
	return "./" + parts[len(parts)-1]
}
