package commands

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Acme Corp", "acme-corp"},
		{"Acme_Corp", "acme-corp"},
		{"  Acme   Corp!! ", "acme-corp"},
		{"ACME", "acme"},
		{"123 Corp", "123-corp"},
		{"--leading---and---trailing--", "leading-and-trailing"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := slugify(c.in); got != c.want {
				t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestComputeProfileName(t *testing.T) {
	cases := []struct {
		workspace, environment, region, want string
	}{
		// ADR-006 examples.
		{"Acme Corp", "prod", "us", "acme-corp-prod"},
		{"Acme Corp", "prod", "eu", "acme-corp-prod-eu"},
		// Region suffix only for non-default region.
		{"Acme", "dev", "us", "acme-dev"},
		{"Acme", "dev", "", "acme-dev"},
		{"Acme", "dev", "jp", "acme-dev-jp"},
	}
	for _, c := range cases {
		got := computeProfileName(c.workspace, c.environment, c.region)
		if got != c.want {
			t.Errorf("computeProfileName(%q,%q,%q) = %q, want %q",
				c.workspace, c.environment, c.region, got, c.want)
		}
	}
}
