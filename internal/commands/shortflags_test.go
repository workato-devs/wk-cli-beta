package commands

import (
	"testing"
)

// TestShortFlagsResolveOnAuthLogin pins the short-flag wiring added for
// issue #32. A missing or renamed short flag would surface as a Lookup
// miss, so this guards against silent regressions in the flag table.
func TestShortFlagsResolveOnAuthLogin(t *testing.T) {
	want := map[string]string{
		"n": "name",
		"w": "workspace",
		"e": "environment",
		"r": "region",
		"t": "token",
		"f": "force",
	}
	cmd := newAuthLoginCmd()
	for short, long := range want {
		if f := cmd.Flags().ShorthandLookup(short); f == nil {
			t.Errorf("auth login: -%s (expected alias for --%s) not registered", short, long)
		} else if f.Name != long {
			t.Errorf("auth login: -%s aliases --%s, want --%s", short, f.Name, long)
		}
	}
}

func TestShortFlagsResolveOnInit(t *testing.T) {
	want := map[string]string{
		"n": "name",
		"s": "server-path",
		"l": "local-path",
		"o": "overwrite",
	}
	cmd := newInitCmd()
	for short, long := range want {
		if f := cmd.Flags().ShorthandLookup(short); f == nil {
			t.Errorf("init: -%s (expected alias for --%s) not registered", short, long)
		} else if f.Name != long {
			t.Errorf("init: -%s aliases --%s, want --%s", short, f.Name, long)
		}
	}
}

// Regression: init must NOT declare its own --profile — a local flag
// with that name would shadow the persistent root --profile, silently
// disabling `wk init -p foo`. The only correct source of init's profile
// is the persistent root flag (flagProfile).
func TestInitDoesNotShadowPersistentProfileFlag(t *testing.T) {
	initCmd := newInitCmd()
	if f := initCmd.Flags().Lookup("profile"); f != nil {
		t.Errorf("init must not declare a local --profile flag (would shadow persistent -p/--profile); found local %q", f.Name)
	}

	// Wire init under a root and confirm -p parses through to flagProfile.
	resetGlobalFlags(t)
	root := NewRootCmd()
	root.AddCommand(initCmd)
	root.SetArgs([]string{"init", "-p", "from-short-flag", "--help"})
	// --help short-circuits execution before validation runs, so we
	// can inspect the parsed value without needing a real profile.
	if err := root.Execute(); err != nil {
		t.Fatalf("init --help: %v", err)
	}
	if flagProfile != "from-short-flag" {
		t.Errorf("flagProfile = %q, want %q (persistent -p did not propagate to init)", flagProfile, "from-short-flag")
	}
}

// Global persistent flags live on the root command.
func TestShortFlagsResolveOnRoot(t *testing.T) {
	want := map[string]string{
		"j": "json",
		"q": "quiet",
		"p": "profile",
	}
	root := NewRootCmd()
	for short, long := range want {
		if f := root.PersistentFlags().ShorthandLookup(short); f == nil {
			t.Errorf("root: -%s (expected alias for --%s) not registered", short, long)
		} else if f.Name != long {
			t.Errorf("root: -%s aliases --%s, want --%s", short, f.Name, long)
		}
	}
}

// -v is deliberately unassigned because --verify (init) and --verbose
// both have a legitimate claim. Lock that in so nobody accidentally
// grabs it later and creates the ambiguity.
func TestShortFlagV_NotAssigned(t *testing.T) {
	root := NewRootCmd()
	if f := root.PersistentFlags().ShorthandLookup("v"); f != nil {
		t.Errorf("-v should not be a global short flag (conflicts with --verify); got --%s", f.Name)
	}
	initCmd := newInitCmd()
	if f := initCmd.Flags().ShorthandLookup("v"); f != nil {
		t.Errorf("init -v should not be assigned; got --%s", f.Name)
	}
}
