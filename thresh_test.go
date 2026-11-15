package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtureSeams mirrors testdata/fixture/.thresh/thresh.yaml. Kinds are exercised
// directly through the uniform contract, so these tests need git but not cue.
var fixtureSeams = map[string]seam{
	"lineages":       {Kind: "published-language", Paths: []string{"schemas/*.cue"}, Format: "thema"},
	"verdicts":       {Kind: "published-language", Paths: []string{"corpus/*.txtar"}, Format: "golden", Section: "out", Headline: true},
	"kernel":         {Kind: "shared-kernel", Paths: []string{"kernel.cue"}, Consult: []string{"every consumer of #Kernel", "the Go binding"}},
	"cue-module":     {Kind: "shared-kernel", Paths: []string{"cue.mod/module.cue"}},
	"cue-dependency": {Kind: "shared-kernel", Paths: []string{"go.mod"}, Watch: []string{"cuelang.org/go"}},
	"build":          {Kind: "choke-point", Paths: []string{"*.go"}, Type: "guarded", OnlyIn: []string{"Build"}},
	"checks":         {Kind: "declared-set", Paths: []string{"bind.go"}, Funcs: "check*", Declared: map[string]string{"checkOne": "first declared check", "checkTwo": "second declared check"}},
	"shapes":         {Kind: "declared-set", Paths: []string{"shapes.cue"}, Defs: "*#*", Declared: map[string]string{"#Shape": "closed shape", "#Edge": "closed edge"}},
	"binding":        {Kind: "references-resolve", Go: []string{"*.go"}, Cue: []string{"*.cue"}},
}

// Every kind has at least one loud scenario and one silence scenario. The
// silence scenarios are not optional: a kind you can't picture staying quiet is
// one nobody would leave switched on. Each scenario runs on a fresh fixture repo.
func TestScenarios(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   []string // substrings of the rendered signals; empty => expect silence
	}{
		{
			"comment-only edit to the kernel is silent",
			func(t *testing.T, dir string) { appendTo(t, dir, "kernel.cue", "\n// just a comment\n") },
			nil,
		},
		{
			"breaking major bump is loud",
			func(t *testing.T, dir string) {
				replace(t, dir, "schemas/contract.cue",
					"{version: [1, 0], schema: {b: int}},",
					"{version: [1, 0], schema: {b: int}},\n\t\t{version: [2, 0], schema: {c: bool}},")
			},
			[]string{"[published-language: lineages]", "breaking change", "[1,0] → [2,0]"},
		},
		{
			"compatible minor is a note",
			func(t *testing.T, dir string) {
				replace(t, dir, "schemas/contract.cue",
					"{version: [1, 0], schema: {b: int}},",
					"{version: [1, 0], schema: {b: int}},\n\t\t{version: [1, 1], schema: {b: int, d?: string}},")
			},
			[]string{"[published-language: lineages]", "compatible change", "note"},
		},
		{
			"comment-only edit to the contract is silent",
			func(t *testing.T, dir string) { appendTo(t, dir, "schemas/contract.cue", "\n// nothing to see\n") },
			nil,
		},
		{
			"a contract file the format reads no versions from is noted as a misconfiguration",
			func(t *testing.T, dir string) {
				write(t, dir, "schemas/notes.cue", "package fixture\n\nnotes: \"no versions here\"\n")
			},
			[]string{"[published-language: lineages] schemas/notes.cue", "found no versions", "note"},
		},
		{
			"a moved recorded headline is loud",
			func(t *testing.T, dir string) {
				replace(t, dir, "corpus/tightened.txtar", "-- out --\nincompatible\n", "-- out --\ncompatible\n")
			},
			[]string{"[published-language: verdicts]", "step-back", `headline moved: "incompatible" → "compatible"`},
		},
		{
			"reworded output under an unchanged headline is a note",
			func(t *testing.T, dir string) {
				replace(t, dir, "corpus/tightened.txtar", "(rule narrowed)", "(rule disjunction-narrowed)")
			},
			[]string{"[published-language: verdicts]", "note", `reworded; headline "incompatible" unchanged`},
		},
		{
			"an input-only edit with the same recorded output is a note",
			func(t *testing.T, dir string) {
				replace(t, dir, "corpus/tightened.txtar", "s: \"a\" | \"b\"", "s: \"a\" | \"c\"")
			},
			[]string{"[published-language: verdicts]", "note", "recorded output unchanged"},
		},
		{
			"a new recorded output is a note that the contract grew",
			func(t *testing.T, dir string) {
				write(t, dir, "corpus/widened.txtar", "-- old.cue --\ns: \"a\"\n-- new.cue --\ns: string\n-- out --\ncompatible\n")
			},
			[]string{"[published-language: verdicts] corpus/widened.txtar", "note", `contract grew: new recorded output (headline "compatible")`},
		},
		{
			"a removed recorded output is loud",
			func(t *testing.T, dir string) {
				if err := os.Remove(filepath.Join(dir, "corpus", "tightened.txtar")); err != nil {
					t.Fatal(err)
				}
			},
			[]string{"[published-language: verdicts] corpus/tightened.txtar", "step-back", "recorded output removed"},
		},
		{
			"a golden file without the declared section is noted as a misconfiguration",
			func(t *testing.T, dir string) {
				write(t, dir, "corpus/broken.txtar", "-- old.cue --\ns: string\n-- output --\ncompatible\n")
			},
			[]string{"[published-language: verdicts] corpus/broken.txtar", "note", `no "out" section`},
		},
		{
			"whitespace-only edit to a golden file is silent",
			func(t *testing.T, dir string) {
				replace(t, dir, "corpus/tightened.txtar", "s: string\n", "s:   string\n")
			},
			nil,
		},
		{
			"construction outside the choke point is loud",
			func(t *testing.T, dir string) {
				write(t, dir, "sneaky.go", "package fixture\n\nfunc Sneaky() *guarded { return &guarded{} }\n")
			},
			[]string{"[choke-point: build]", "Sneaky()", "guarded"},
		},
		{
			"construction inside the sanctioned function is silent",
			func(t *testing.T, dir string) {
				// parse-only fixture: a construction inside a function named Build
				write(t, dir, "sanctioned.go", "package fixture\n\nfunc Build() *guarded { return &guarded{v: 1} }\n")
			},
			nil,
		},
		{
			"type switch and assertion are not construction",
			func(t *testing.T, dir string) {
				write(t, dir, "kindof.go", "package fixture\n\nfunc kindOf(x any) string {\n\tswitch x.(type) {\n\tcase *guarded:\n\t\treturn \"g\"\n\t}\n\t_, _ = x.(*guarded)\n\treturn \"\"\n}\n")
			},
			nil,
		},
		{
			"dependency bump and cue.mod change are both loud",
			func(t *testing.T, dir string) {
				replace(t, dir, "go.mod", "cuelang.org/go v0.5.0", "cuelang.org/go v0.11.0")
				appendTo(t, dir, "cue.mod/module.cue", "language: version: \"v0.14.0\"\n")
			},
			[]string{"[shared-kernel: cue-dependency]", "cuelang.org/go v0.11.0", "[shared-kernel: cue-module]"},
		},
		{
			"unrelated dependency is silent",
			func(t *testing.T, dir string) {
				replace(t, dir, "go.mod", "\tcuelang.org/go v0.5.0\n", "\tcuelang.org/go v0.5.0\n\texample.com/other v1.0.0\n")
			},
			nil,
		},
		{
			"field added to the kernel names the definition and the co-owners",
			func(t *testing.T, dir string) {
				replace(t, dir, "kernel.cue", "\tname: string\n", "\tname: string\n\tdescription?: string\n")
			},
			[]string{"[shared-kernel: kernel]", "#Kernel", "Consult: every consumer of #Kernel; the Go binding"},
		},
		{
			"undeclared check added is loud",
			func(t *testing.T, dir string) {
				appendTo(t, dir, "bind.go", "\nfunc (m *maybe) checkThree() error { return nil }\n")
			},
			[]string{"[declared-set: checks]", "checkThree"},
		},
		{
			"body edit of a declared check is silent",
			func(t *testing.T, dir string) {
				replace(t, dir, "bind.go", "func (m *maybe) checkOne() error { return nil }",
					"func (m *maybe) checkOne() error {\n\t_ = m\n\treturn nil\n}")
			},
			nil,
		},
		{
			"undeclared CUE definition added, nested, is loud",
			func(t *testing.T, dir string) {
				replace(t, dir, "shapes.cue", "\tname: string\n", "\tname: string\n\t#Helper: string\n")
			},
			[]string{"[declared-set: shapes]", "#Helper"},
		},
		{
			"undeclared hidden CUE definition added is loud",
			func(t *testing.T, dir string) { appendTo(t, dir, "shapes.cue", "\n_#Scratch: {}\n") },
			[]string{"[declared-set: shapes]", "_#Scratch"},
		},
		{
			"regular field added to a declared CUE definition is silent",
			func(t *testing.T, dir string) {
				replace(t, dir, "shapes.cue", "\tname: string\n", "\tname: string\n\tnote?: string\n")
			},
			nil,
		},
		{
			"declared CUE definition removed is a note",
			func(t *testing.T, dir string) {
				replace(t, dir, "shapes.cue", "\n#Edge: {\n\tfrom: string\n\tto:   string\n}\n", "\n")
			},
			[]string{"[declared-set: shapes]", "#Edge no longer exists", "note"},
		},
		{
			"declared check removed is a note",
			func(t *testing.T, dir string) {
				replace(t, dir, "bind.go", "func (m *maybe) checkTwo() error { return nil }\n", "")
			},
			[]string{"[declared-set: checks]", "checkTwo no longer exists", "note"},
		},
		{
			"renamed CUE label breaks Go references, including an untouched file",
			func(t *testing.T, dir string) {
				replace(t, dir, "kernel.cue", "\tlenses: [...#Lens]", "\tlensList: [...#Lens]")
			},
			[]string{"[references-resolve: binding]", `"lenses"`, "bind.go", "other.go", "[shared-kernel: kernel]"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixture(t)
			tc.mutate(t, dir)
			commitAll(t, dir, "change")
			got := check(t, dir)
			if len(tc.want) == 0 {
				if got != "" {
					t.Fatalf("expected silence, got:\n%s", got)
				}
				return
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
		})
	}
}

// TestAnnotationsValidate exercises kindFields through loadConfig: the fixture
// annotations are accepted and match fixtureSeams; bad ones are rejected with an
// error naming the file, line and problem.
func TestAnnotationsValidate(t *testing.T) {
	if !reflect.DeepEqual(sortedKeys(kindFields), sortedKeys(kinds)) {
		t.Fatalf("kindFields declares %v, kinds implements %v", sortedKeys(kindFields), sortedKeys(kinds))
	}
	dir := fixture(t)
	cfg, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range sortedKeys(fixtureSeams) {
		got, ok := cfg.Seams[name]
		if !ok {
			t.Errorf("fixture .thresh/thresh.yaml lacks seam %q declared in fixtureSeams", name)
			continue
		}
		got.Purpose = "" // fixtureSeams leaves purposes out
		if !reflect.DeepEqual(got, fixtureSeams[name]) {
			t.Errorf("seam %q: fixture .thresh/thresh.yaml has %+v, fixtureSeams has %+v", name, got, fixtureSeams[name])
		}
	}
	for _, name := range sortedKeys(cfg.Seams) {
		if _, ok := fixtureSeams[name]; !ok {
			t.Errorf("fixtureSeams lacks seam %q declared in fixture .thresh/thresh.yaml", name)
		}
	}

	for _, c := range []struct {
		name  string
		files map[string]string
		want  []string // every one must appear in the error
	}{
		{"unknown kind", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: frobnicate\n    purpose: p\n"},
			[]string{"invalid .thresh annotations", ".thresh/thresh.yaml:3", `"frobnicate"`}},
		{"missing kind", map[string]string{"thresh.yaml": "seams:\n  oops:\n    purpose: p\n"},
			[]string{"kind is required"}},
		// Every kind is closed, so a typo is rejected rather than ignored.
		{"misspelled field", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: choke-point\n    purpose: p\n    paths: [x.go]\n    type: x\n    onlyIn: [f]\n    onlyInn: [g]\n"},
			[]string{".thresh/thresh.yaml:8", `"onlyInn"`, "choke-point"}},
		{"another kind's field", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: shared-kernel\n    purpose: p\n    paths: [x.go]\n    onlyIn: [f]\n"},
			[]string{`"onlyIn"`, "shared-kernel"}},
		{"missing required field", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: choke-point\n    purpose: p\n    paths: [x.go]\n    onlyIn: [f]\n"},
			[]string{"type is required for kind choke-point"}},
		// Every seam must say why it exists.
		{"missing purpose", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: shared-kernel\n    paths: [x.go]\n"},
			[]string{"purpose is required"}},
		{"empty purpose", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: shared-kernel\n    purpose: \"\"\n    paths: [x.go]\n"},
			[]string{"purpose must say why"}},
		{"wrong type", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: shared-kernel\n    purpose: p\n    paths: x.go\n"},
			[]string{".thresh/thresh.yaml:", "[]string"}},
		{"unknown format", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: published-language\n    purpose: p\n    paths: [x]\n    format: protobuf\n"},
			[]string{`unknown format "protobuf"`}},
		{"golden field on thema", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: published-language\n    purpose: p\n    paths: [x]\n    format: thema\n    section: out\n"},
			[]string{"section applies only to format golden"}},
		{"both funcs and defs", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: declared-set\n    purpose: p\n    paths: [x]\n    funcs: \"check*\"\n    defs: \"#*\"\n"},
			[]string{"exactly one of funcs"}},
		{"neither funcs nor defs", map[string]string{"thresh.yaml": "seams:\n  oops:\n    kind: declared-set\n    purpose: p\n    paths: [x]\n"},
			[]string{"exactly one of funcs"}},
		// Files compose by seam name; declaring one twice is never a merge.
		{"seam declared twice", map[string]string{
			"a.yaml": "seams:\n  oops:\n    kind: shared-kernel\n    purpose: p\n    paths: [x.go]\n",
			"b.yml":  "seams:\n  oops:\n    kind: shared-kernel\n    purpose: q\n    paths: [y.go]\n"},
			[]string{".thresh/b.yml:2", `seam "oops" is already declared at .thresh/a.yaml:2`}},
		{"unknown top-level key", map[string]string{"thresh.yaml": "seam:\n  oops: {}\n"},
			[]string{`unknown top-level key "seam"`}},
		// Every problem is reported, not just the first.
		{"several problems", map[string]string{"thresh.yaml": "seams:\n  a:\n    kind: frobnicate\n  b:\n    kind: shared-kernel\n    paths: [x]\n"},
			[]string{`"frobnicate"`, `seam "b": purpose is required`}},
		// CUE config is no longer read; a leftover is an error, not silence.
		{"leftover cue config", map[string]string{"thresh.cue": "package thresh\n"},
			[]string{".thresh/thresh.cue", "YAML now"}},
	} {
		bad := t.TempDir()
		for f, body := range c.files {
			write(t, bad, filepath.Join(".thresh", f), body)
		}
		_, err := loadConfig(bad)
		if err == nil {
			t.Errorf("%s: expected an error", c.name)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: error lacks %q:\n%v", c.name, w, err)
			}
		}
	}

	// A file with every seam still commented out declares nothing, quietly.
	empty := t.TempDir()
	write(t, empty, ".thresh/thresh.yaml", "# nothing yet\nseams:\n  # oops:\n  #   kind: shared-kernel\n")
	if cfg, err := loadConfig(empty); err != nil || len(cfg.Seams) != 0 {
		t.Errorf("commented-out config: expected no seams and no error, got %v, %v", cfg, err)
	}

	// explain renders each seam's purpose ahead of its obligation.
	var b strings.Builder
	explain(&b, cfg)
	if !strings.Contains(b.String(), "- **kernel** (`shared-kernel`): #Kernel is what both the CUE and Go sides stand on\n  kernel.cue is co-owned kernel.") {
		t.Errorf("explain lacks the kernel seam's purpose:\n%s", b.String())
	}
}

func TestNoSeamsNeverTouchesGit(t *testing.T) {
	dir := t.TempDir() // not a git repo, no .thresh/
	if cfg, err := loadConfig(dir); err != nil || len(cfg.Seams) != 0 {
		t.Fatalf("expected empty config without error, got %v, %v", cfg, err)
	}
}

func TestInitScaffold(t *testing.T) {
	dir := t.TempDir()
	if err := initRepo(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".thresh", "thresh.yaml")
	cfg, err := loadConfig(dir)
	if err != nil || len(cfg.Seams) != 0 {
		t.Fatalf("fresh scaffold: expected no seams and no error, got %v, %v", cfg, err)
	}
	if err := initRepo(dir); err == nil {
		t.Fatal("expected init to refuse to overwrite an existing .thresh/")
	}

	// Uncommented as its intro says, every example is a valid seam, and between
	// them they cover every kind.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(b), "\n  # ", "\n  ")), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{}
	for _, s := range cfg.Seams {
		covered[s.Kind] = true
	}
	for _, k := range sortedKeys(kindFields) {
		if !covered[k] {
			t.Errorf("init scaffold has no example of kind %s", k)
		}
	}
}

// --- helpers --------------------------------------------------------------------

// fixture copies testdata/fixture into a fresh temp git repo and commits it.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		dst := filepath.Join(dir, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "thresh@test")
	git(t, dir, "config", "user.name", "thresh test")
	git(t, dir, "config", "commit.gpgsign", "false")
	commitAll(t, dir, "base")
	return dir
}

// check runs every fixture seam against HEAD~1..HEAD and renders the signals
// in a greppable form.
func check(t *testing.T, dir string) string {
	t.Helper()
	cs, err := newChangeset(dir, "HEAD~1..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, name := range sortedKeys(fixtureSeams) {
		s := fixtureSeams[name]
		for _, sig := range kinds[s.Kind].assess(name, s, cs) {
			b.WriteString("[" + sig.Kind + ": " + sig.Seam + "] " + sig.File + " " + sig.Level + ": " + sig.Message + "\n")
		}
	}
	return b.String()
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", msg)
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, dir, rel, s string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, rel, string(b)+s)
}

// replace substitutes old with new exactly once, failing loudly if old is absent
// so fixture drift is caught rather than silently producing a vacuous scenario.
func replace(t *testing.T, dir, rel, old, new string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s: fixture drift — %q not found", rel, old)
	}
	write(t, dir, rel, strings.Replace(string(b), old, new, 1))
}
