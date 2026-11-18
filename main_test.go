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

// A working-tree check reads every kind of difference from HEAD in one status:
// staged, unstaged, deleted, untracked, removed from the index but kept on disk,
// and paths git would otherwise quote.
func TestWorktreeChangedFiles(t *testing.T) {
	dir := fixture(t)
	replace(t, dir, "bind.go", "return &guarded{}", "return &guarded{v: 2}")
	replace(t, dir, "kernel.cue", "name: string", "name: string\n\tlabel: string")
	git(t, dir, "add", "kernel.cue")
	if err := os.Remove(filepath.Join(dir, "shapes.cue")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "rm", "-q", "--cached", "other.go")
	write(t, dir, "notes and more.txt", "x\n")
	write(t, dir, "café/menu.txt", "x\n")

	before := gitCalls
	cs, err := newChangeset(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if n := gitCalls - before; n != 1 {
		t.Errorf("reading the working tree's changes ran git %d times, want 1", n)
	}
	got := map[string]bool{}
	for _, p := range cs.changed {
		if got[p] {
			t.Errorf("%s listed twice", p)
		}
		got[p] = true
	}
	for _, p := range []string{"bind.go", "kernel.cue", "shapes.cue", "other.go", "notes and more.txt", "café/menu.txt"} {
		if !got[p] {
			t.Errorf("%s missing from changed files %q", p, cs.changed)
		}
	}
	if len(got) != 6 {
		t.Errorf("changed files = %q, want exactly 6", cs.changed)
	}
	head, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if want := strings.TrimSpace(string(head)); cs.baseSHA != want || cs.base != want {
		t.Errorf("base %q, baseSHA %q, want both %s", cs.base, cs.baseSHA, want)
	}
}

func TestParseStatus(t *testing.T) {
	out := strings.Join([]string{
		"# branch.oid 1111111111111111111111111111111111111111",
		"# branch.head main",
		"1 .M N... 100644 100644 100644 aaaa aaaa a file.go",
		"u UU N... 100644 100644 100644 100644 bbbb cccc dddd con flict.cue",
		"? new.txt",
		"",
	}, "\x00")
	oid, paths, err := parseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if oid != "1111111111111111111111111111111111111111" || strings.Join(paths, "|") != "a file.go|con flict.cue|new.txt" {
		t.Errorf("got %q, %q", oid, paths)
	}
	if oid, _, _ := parseStatus("# branch.oid (initial)\x00# branch.head main\x00? a\x00"); oid != "" {
		t.Errorf("unborn HEAD gave oid %q", oid)
	}
	for _, bad := range []string{"1 .M N... 100644 100644 100644 aaaa aaaa\x00", "2 R. N... 100644 100644 100644 a a R100 new\x00old\x00"} {
		if _, _, err := parseStatus(bad); err == nil {
			t.Errorf("parseStatus(%q) accepted an entry it cannot read", bad)
		}
	}
}

// A check that finds no taps needs neither the repo's identity nor the tap log:
// one git run for a change outside every seam, and no log to fail on. With a
// tap, the same unusable log is reported, so the silence isn't vacuous.
func TestNoTapsReadNoIdentityOrLog(t *testing.T) {
	dir := fixture(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THRESH_LOG", filepath.Join(blocker, "taps.jsonl")) // unusable: its parent is a file
	run := func() (stdout, stderr string, calls int) {
		t.Helper()
		var o, e strings.Builder
		before := gitCalls
		if err := runCheck(options{repo: dir, session: "s", deliver: true}, config{Seams: fixtureSeams}, &o, &e); err != nil {
			t.Fatal(err)
		}
		return o.String(), e.String(), gitCalls - before
	}

	write(t, dir, "notes.txt", "outside every seam\n")
	if out, errs, calls := run(); out != "" || errs != "" || calls != 1 {
		t.Errorf("change outside the seams: %d git runs (want 1), stdout %q, stderr %q", calls, out, errs)
	}

	// status, the changed files at base, their hunks
	replace(t, dir, "bind.go", "// Literal references into the CUE kernel.", "// Literal references into the CUE kernel, by path.")
	if out, errs, calls := run(); out != "" || errs != "" || calls != 3 {
		t.Errorf("comment-only Go edit: %d git runs (want 3), stdout %q, stderr %q", calls, out, errs)
	}

	replace(t, dir, "kernel.cue", "name: string", "name: string\n\tlabel: string")
	if out, errs, _ := run(); !strings.Contains(out, "## Thresh") || !strings.Contains(errs, "tap log not written") {
		t.Errorf("kernel change: stdout %q, stderr %q", out, errs)
	}
}
