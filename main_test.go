package main

import "testing"

func TestVersionOf(t *testing.T) {
	for _, c := range []struct{ version, rev, want string }{
		// a release binary, built from a checkout at the tag
		{"v0.1.0", "b5b06913fd77", "v0.1.0 (b5b06913fd77)"},
		// go install …@v0.1.0: built from the module cache, no VCS stamp
		{"v0.1.0", "v0.1.0", "v0.1.0"},
		// a checkout past the tag: the pseudo-version already names the commit
		{"v0.1.1-0.20261115210000-b5b06913fd77+dirty", "b5b06913fd77+dirty", "v0.1.1-0.20261115210000-b5b06913fd77+dirty"},
		{"(devel)", "b5b06913fd77+dirty", "(devel) (b5b06913fd77+dirty)"},
		{"", "b5b06913fd77", "b5b06913fd77"},
	} {
		if got := versionOf(c.version, c.rev); got != c.want {
			t.Errorf("versionOf(%q, %q) = %q, want %q", c.version, c.rev, got, c.want)
		}
	}
}
