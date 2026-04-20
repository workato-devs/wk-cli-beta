package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSyncFlag(t *testing.T) {
	tests := []struct {
		in         string
		wantServer string
		wantLocal  string
		wantErr    bool
	}{
		{"Recipes/Slack", "Recipes/Slack", "./Slack", false},
		{"Recipes/Slack:./code/slack", "Recipes/Slack", "./code/slack", false},
		{"Recipes/Slack:", "Recipes/Slack", "./Slack", false},
		{"All projects/Recipes:./rx", "All projects/Recipes", "./rx", false},
		{"  Recipes/Slack  ", "Recipes/Slack", "./Slack", false},
		{":/missing-server", "", "", true},
		{"", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			e, err := parseSyncFlag(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSyncFlag(%q) err = nil, want err", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSyncFlag(%q) err = %v", tc.in, err)
			}
			if e.ServerPath != tc.wantServer || e.LocalPath != tc.wantLocal {
				t.Errorf("parseSyncFlag(%q) = {%q, %q}, want {%q, %q}",
					tc.in, e.ServerPath, e.LocalPath, tc.wantServer, tc.wantLocal)
			}
		})
	}
}

func TestDefaultLocalPathForServerPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Recipes/Slack", "./Slack"},
		{"Recipes", "./Recipes"},
		{"All projects/Recipes/Slack", "./Slack"},
		{"All projects", "."},
		{"/Recipes/Slack/", "./Slack"},
		{"", "."},
	}
	for _, tc := range tests {
		if got := defaultLocalPathForServerPath(tc.in); got != tc.want {
			t.Errorf("defaultLocalPathForServerPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// chdirToTempRoot sets up a temp dir, cd's into it, and returns the dir plus
// a deferred restore so helper tests can use relative paths without tripping
// the absolute-path guard in ValidateLocalPath.
func chdirToTempRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}

func TestAssembleSyncEntries_DiscoveryMode(t *testing.T) {
	_ = chdirToTempRoot(t)
	for _, sub := range []string{"alpha", "beta", ".hidden"} {
		if err := os.MkdirAll(filepath.Join("recipes", sub), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	if err := os.WriteFile(filepath.Join("recipes", "README.md"), []byte("x"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	entries, err := AssembleSyncEntries(&SyncEntryFlags{ProjectsDir: "recipes"}, ".")
	if err != nil {
		t.Fatalf("AssembleSyncEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (%+v)", len(entries), entries)
	}
	names := map[string]string{}
	for _, e := range entries {
		names[e.ServerPath] = e.LocalPath
	}
	if names["alpha"] != filepath.Join("recipes", "alpha") {
		t.Errorf("alpha local = %q, want %q", names["alpha"], filepath.Join("recipes", "alpha"))
	}
	if names["beta"] != filepath.Join("recipes", "beta") {
		t.Errorf("beta local = %q, want %q", names["beta"], filepath.Join("recipes", "beta"))
	}
	if _, ok := names[".hidden"]; ok {
		t.Errorf("hidden dir was not skipped: %+v", entries)
	}
}

func TestAssembleSyncEntries_DiscoveryEmptyErrors(t *testing.T) {
	_ = chdirToTempRoot(t)
	if err := os.MkdirAll("empty-recipes", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := AssembleSyncEntries(&SyncEntryFlags{ProjectsDir: "empty-recipes"}, ".")
	if err == nil || !strings.Contains(err.Error(), "no non-hidden subdirectories") {
		t.Errorf("err = %v, want 'no non-hidden subdirectories'", err)
	}
}

func TestAssembleSyncEntries_PrefixOnlyMode(t *testing.T) {
	_ = chdirToTempRoot(t)
	entries, err := AssembleSyncEntries(&SyncEntryFlags{
		Projects:    []string{"alpha", "beta"},
		ProjectsDir: "workato/recipes",
	}, ".")
	if err != nil {
		t.Fatalf("AssembleSyncEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (%+v)", len(entries), entries)
	}
	if entries[0].ServerPath != "alpha" || entries[0].LocalPath != filepath.Join("workato/recipes", "alpha") {
		t.Errorf("entry 0 = %+v", entries[0])
	}
	if entries[1].ServerPath != "beta" || entries[1].LocalPath != filepath.Join("workato/recipes", "beta") {
		t.Errorf("entry 1 = %+v", entries[1])
	}
}

func TestAssembleSyncEntries_ProjectRejectsSlashesAndColons(t *testing.T) {
	_ = chdirToTempRoot(t)
	cases := []string{"Team/Alpha", "colon:name", "windows\\style"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := AssembleSyncEntries(&SyncEntryFlags{
				Projects:    []string{name},
				ProjectsDir: ".",
			}, ".")
			if err == nil || !strings.Contains(err.Error(), "bare name") {
				t.Errorf("err = %v, want 'bare name'", err)
			}
		})
	}
}

func TestAssembleSyncEntries_RejectsDuplicateTuple(t *testing.T) {
	_ = chdirToTempRoot(t)
	_, err := AssembleSyncEntries(&SyncEntryFlags{
		Projects:    []string{"alpha", "alpha"},
		ProjectsDir: ".",
	}, ".")
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Errorf("err = %v, want 'declared twice'", err)
	}
}

func TestAssembleSyncEntries_ConflictSameServerDifferentLocal(t *testing.T) {
	_ = chdirToTempRoot(t)
	_, err := AssembleSyncEntries(&SyncEntryFlags{
		Projects:    []string{"alpha"},
		ProjectsDir: ".",
		Syncs:       []string{"alpha:somewhere-else"},
	}, ".")
	if err == nil || !strings.Contains(err.Error(), "conflicting local paths") {
		t.Errorf("err = %v, want 'conflicting local paths'", err)
	}
}

func TestAssembleSyncEntries_RejectsTraversalInSyncFlag(t *testing.T) {
	_ = chdirToTempRoot(t)
	_, err := AssembleSyncEntries(&SyncEntryFlags{
		ProjectsDir: ".",
		Syncs:       []string{"alpha:../escape"},
	}, ".")
	if err == nil || !strings.Contains(err.Error(), "escapes the project root") {
		t.Errorf("err = %v, want 'escapes the project root'", err)
	}
}

func TestAssembleSyncEntries_RejectsTraversalInProjectsDir(t *testing.T) {
	_ = chdirToTempRoot(t)
	_, err := AssembleSyncEntries(&SyncEntryFlags{
		ProjectsDir: "../outside",
	}, ".")
	if err == nil || !strings.Contains(err.Error(), "--projects-dir") {
		t.Errorf("err = %v, want '--projects-dir'", err)
	}
}
