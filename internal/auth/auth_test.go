package auth

import (
	"strings"
	"testing"
)

func TestRegionIsValid(t *testing.T) {
	tests := []struct {
		region Region
		valid  bool
	}{
		{RegionUS, true},
		{RegionEU, true},
		{RegionJP, true},
		{RegionAU, true},
		{RegionSG, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.region.IsValid(); got != tt.valid {
			t.Errorf("Region(%q).IsValid() = %v, want %v", tt.region, got, tt.valid)
		}
	}
}

func TestValidRegions(t *testing.T) {
	regions := ValidRegions()
	if len(regions) != 6 {
		t.Errorf("ValidRegions() len = %d, want 6", len(regions))
	}
}

func TestProfileManager(t *testing.T) {
	dir := t.TempDir()
	pm := &ProfileManager{Dir: dir}

	// List should be empty
	profiles, err := pm.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected empty profiles, got %d", len(profiles))
	}

	// Save a profile
	p := &Profile{
		Name:        "test",
		Workspace:   "acme-corp",
		Environment: "dev",
		Region:      RegionUS,
		StoreType:   StoreKeychain,
		BaseURL:     "https://www.workato.com",
	}
	if err := pm.SaveProfile(p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	// Get the profile
	got, err := pm.GetProfile("test")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("Name = %q, want %q", got.Name, "test")
	}
	if got.Workspace != "acme-corp" {
		t.Errorf("Workspace = %q, want %q", got.Workspace, "acme-corp")
	}
	if got.Environment != "dev" {
		t.Errorf("Environment = %q, want %q", got.Environment, "dev")
	}

	// Set active
	if err := pm.SetActiveProfile("test"); err != nil {
		t.Fatalf("SetActiveProfile: %v", err)
	}
	active, err := pm.GetActiveProfile()
	if err != nil {
		t.Fatalf("GetActiveProfile: %v", err)
	}
	if active != "test" {
		t.Errorf("active = %q, want %q", active, "test")
	}

	// Delete
	if err := pm.DeleteProfile("test"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	profiles, _ = pm.ListProfiles()
	if len(profiles) != 0 {
		t.Errorf("expected empty after delete, got %d", len(profiles))
	}
}

func TestSaveProfileUniqueness(t *testing.T) {
	dir := t.TempDir()
	pm := &ProfileManager{Dir: dir}

	// Save first profile.
	p1 := &Profile{
		Name:        "dev",
		Workspace:   "acme-corp",
		Environment: "dev",
		Region:      RegionUS,
		StoreType:   StoreKeychain,
		BaseURL:     "https://www.workato.com",
	}
	if err := pm.SaveProfile(p1); err != nil {
		t.Fatalf("SaveProfile(dev): %v", err)
	}

	// Save second profile targeting same tuple — should fail.
	p2 := &Profile{
		Name:        "dev-alias",
		Workspace:   "acme-corp",
		Environment: "dev",
		Region:      RegionUS,
		StoreType:   StoreKeychain,
		BaseURL:     "https://www.workato.com",
	}
	err := pm.SaveProfile(p2)
	if err == nil {
		t.Fatal("expected duplicate target error, got nil")
	}
	if !strings.Contains(err.Error(), "already targets") {
		t.Errorf("error = %q, want duplicate target message", err.Error())
	}

	// Same name update should succeed (updating own profile).
	p1Updated := &Profile{
		Name:        "dev",
		Workspace:   "acme-corp",
		Environment: "dev",
		Region:      RegionUS,
		StoreType:   StoreKeychain,
		BaseURL:     "https://www.workato.com/updated",
	}
	if err := pm.SaveProfile(p1Updated); err != nil {
		t.Fatalf("SaveProfile(update dev): %v", err)
	}

	// Different environment should succeed.
	p3 := &Profile{
		Name:        "prod",
		Workspace:   "acme-corp",
		Environment: "prod",
		Region:      RegionUS,
		StoreType:   StoreKeychain,
		BaseURL:     "https://www.workato.com",
	}
	if err := pm.SaveProfile(p3); err != nil {
		t.Fatalf("SaveProfile(prod): %v", err)
	}

	// Empty workspace/environment should skip uniqueness check (backward compat).
	legacy := &Profile{
		Name:      "legacy",
		Region:    RegionUS,
		StoreType: StoreKeychain,
		BaseURL:   "https://www.workato.com",
	}
	if err := pm.SaveProfile(legacy); err != nil {
		t.Fatalf("SaveProfile(legacy): %v", err)
	}
}

