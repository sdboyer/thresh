package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestMain lets a test run the CLI itself: with THRESH_TEST_MAIN set, the test
// binary is thresh, given those arguments.
func TestMain(m *testing.M) {
	if args, ok := os.LookupEnv("THRESH_TEST_MAIN"); ok {
		os.Args = append([]string{"thresh"}, strings.Fields(args)...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func thresh(t *testing.T, args string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "THRESH_TEST_MAIN="+args)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

// --help is help on every command, never a range; an unknown flag is an error.
func TestHelpAndUnknownFlags(t *testing.T) {
	for _, args := range []string{"--help", "-h", "help", "check --help", "check -h", "verdict --help"} {
		out, code := thresh(t, args)
		if code != 0 || !strings.HasPrefix(out, "usage:") {
			t.Errorf("thresh %s: exit %d, output:\n%s", args, code, out)
		}
	}
	out, code := thresh(t, "check --bogus")
	if code != 2 || !strings.Contains(out, "unknown flag --bogus") || strings.Contains(out, "range must be") {
		t.Errorf("thresh check --bogus: exit %d, output:\n%s", code, out)
	}
}

func checkOutput(t *testing.T, dir string) string {
	t.Helper()
	t.Setenv("THRESH_LOG", filepath.Join(t.TempDir(), "taps.jsonl"))
	var stdout, stderr strings.Builder
	if err := runCheck(options{repo: dir}, config{Seams: fixtureSeams}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	return stdout.String()
}

// A clean working tree says so, and names the range that checks commits HEAD
// has beyond the remote's default branch; with none, it names no range.
func TestCleanTreeNamesTheRange(t *testing.T) {
	origin := fixture(t)
	dir := filepath.Join(t.TempDir(), "clone")
	git(t, origin, "clone", "-q", origin, dir)
	git(t, dir, "config", "user.email", "thresh@test")
	git(t, dir, "config", "user.name", "thresh test")
	git(t, dir, "config", "commit.gpgsign", "false")

	got := checkOutput(t, dir)
	if !strings.Contains(got, "no uncommitted changes") || strings.Contains(got, "ahead") {
		t.Errorf("clean clone, nothing ahead:\n%s", got)
	}

	replace(t, dir, "kernel.cue", "name: string", "name: string\n\tlabel: string")
	commitAll(t, dir, "a committed kernel change")
	got = checkOutput(t, dir)
	if !strings.Contains(got, "1 commit(s) ahead of origin/") || !strings.Contains(got, "..HEAD` checks them") {
		t.Errorf("clean tree, one commit ahead:\n%s", got)
	}
}

// With no commits there is no base to compare against: check says so, exit 0.
func TestNoCommitsYet(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "kernel.cue", "package fixture\n")
	if got := checkOutput(t, dir); !strings.Contains(got, "no commits yet") {
		t.Errorf("unborn HEAD:\n%s", got)
	}
}
