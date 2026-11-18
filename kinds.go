package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// --- The uniform contract every kind satisfies ---------------------------------
//
// Kinds receive a changeset (files with before/after content and changed line
// numbers) and return signals. They never touch git or output. This is the whole
// shared interface; all variability lives in the extractors (extract_*.go).

type changedFile struct {
	Path    string
	Before  string       // content at base; "" if the file is new
	After   string       // content now;    "" if the file was deleted
	Changed map[int]bool // line numbers added/modified in After
}

type signal struct {
	ID      string `json:"id"`      // the tap's identity in the tap log (taplog.go); never set by a kind
	Kind    string `json:"kind"`    // the catalog kind
	Seam    string `json:"seam"`    // the seam's stable name (map key)
	File    string `json:"file"`    // the file that tripped it ("" if seam-wide)
	Level   string `json:"level"`   // step-back | note
	Message string `json:"message"` // the shoulder-tap: a fact about the diff
}

// seam is the union of every per-kind field. kindFields (config.go) says which
// fields each kind takes, and loading enforces it, so kinds just read them.
type seam struct {
	Kind     string            `json:"kind" yaml:"kind"`
	Purpose  string            `json:"purpose" yaml:"purpose"` // every kind: why the seam exists
	Paths    []string          `json:"paths" yaml:"paths"`
	Format   string            `json:"format" yaml:"format"`     // published-language: compat extractor
	Section  string            `json:"section" yaml:"section"`   // published-language golden: txtar section holding the output
	Headline bool              `json:"headline" yaml:"headline"` // published-language golden: first line of the output is its headline
	Consult  []string          `json:"consult" yaml:"consult"`   // shared-kernel: co-owners to consult
	Watch    []string          `json:"watch" yaml:"watch"`       // shared-kernel: only changed lines containing these
	Type     string            `json:"type" yaml:"type"`         // choke-point: the guarded type
	OnlyIn   []string          `json:"onlyIn" yaml:"onlyIn"`     // choke-point: functions allowed to construct it
	Funcs    string            `json:"funcs" yaml:"funcs"`       // declared-set: glob on Go function name
	Defs     string            `json:"defs" yaml:"defs"`         // declared-set: glob on CUE definition name
	Declared map[string]string `json:"declared" yaml:"declared"` // declared-set: name -> reason it is Go-side
	Go       []string          `json:"go" yaml:"go"`             // references-resolve: Go files
	Cue      []string          `json:"cue" yaml:"cue"`           // references-resolve: CUE files
}

type config struct {
	Seams map[string]seam `json:"seams"`
}

type kind interface {
	assess(name string, s seam, cs *changeset) []signal
}

var kinds = map[string]kind{
	"published-language": publishedLanguage{},
	"shared-kernel":      sharedKernel{},
	"choke-point":        chokePoint{},
	"declared-set":       declaredSet{},
	"references-resolve": referencesResolve{},
}

// --- Changeset: the tool's view of the diff, produced from git ------------------

type changeset struct {
	repo     string
	base     string // ref for "before": HEAD in working-tree mode, merge-base(A, B) in range mode
	head     string // ref for "after":  "" in working-tree mode (read disk), B in range mode
	diffspec string // what to pass to `git diff`
	baseSHA  string // base resolved to a commit
	headSHA  string // head resolved to a commit; "" in working-tree mode
	worktree bool
	changed  []string
	cache    map[string]*changedFile

	// Every git read below runs once per changeset, however many files and seams
	// ask: a process costs far more than the bytes it returns.
	lists map[string][]string  // ref ("" = working tree) -> every file in it
	blob  map[string]string    // "ref:path" -> content, "" where absent
	diffs map[string]*fileDiff // path -> its hunks, from one diff of the changeset
}

// gitCalls counts git processes run, for tests.
var gitCalls int

func newChangeset(repo, rng string) (*changeset, error) {
	cs := &changeset{repo: repo, cache: map[string]*changedFile{}, lists: map[string][]string{}, blob: map[string]string{}}
	if rng == "" {
		// One status read names HEAD and every file that differs from it, staged,
		// unstaged or untracked. The base is that commit, not "HEAD", so every
		// later read sees the same commit even if HEAD moves meanwhile.
		out, err := cs.git("--no-optional-locks", "status", "--porcelain=v2", "-z",
			"--no-renames", "--untracked-files=all", "--branch")
		if err != nil {
			return nil, fmt.Errorf("git status failed: %v", gitErr(err))
		}
		oid, changed, err := parseStatus(out)
		if err != nil {
			return nil, err
		}
		if oid == "" {
			return nil, errNoCommits
		}
		cs.worktree, cs.base, cs.baseSHA, cs.diffspec, cs.changed = true, oid, oid, oid, changed
		return cs, nil
	}
	// A..B and A...B both mean the commits on B since it left A, diffed from
	// their merge base: what moved on A meanwhile is not this change.
	a, b, ok := strings.Cut(rng, "...")
	if !ok {
		a, b, ok = strings.Cut(rng, "..")
	}
	if !ok || a == "" || b == "" {
		return nil, fmt.Errorf("range must be A..B, got %q", rng)
	}
	for _, ref := range []string{a, b} {
		if _, err := cs.resolve(ref); err != nil {
			return nil, err
		}
	}
	mb, err := cs.git("merge-base", a, b)
	if err != nil {
		return nil, fmt.Errorf("%s and %s share no history, so there is no merge base to diff from", a, b)
	}
	cs.base, cs.head = strings.TrimSpace(mb), b
	cs.baseSHA, cs.diffspec = cs.base, cs.base+".."+b
	out, err := cs.git("diff", "--name-only", "-z", cs.diffspec)
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %v", gitErr(err))
	}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			cs.changed = append(cs.changed, p)
		}
	}
	if cs.headSHA, err = cs.resolve(cs.head); err != nil {
		return nil, err
	}
	return cs, nil
}

// parseStatus reads `git status --porcelain=v2 -z --branch --no-renames`: the
// commit HEAD names ("" when it names none yet) and every path the status lists,
// once: a path removed from the index but kept on disk is listed both ways.
func parseStatus(out string) (oid string, paths []string, err error) {
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	for _, rec := range strings.Split(out, "\x00") {
		var f []string
		switch {
		case rec == "":
			continue
		case strings.HasPrefix(rec, "# branch.oid "):
			if oid = strings.TrimPrefix(rec, "# branch.oid "); oid == "(initial)" {
				oid = ""
			}
			continue
		case strings.HasPrefix(rec, "# "):
			continue
		case strings.HasPrefix(rec, "? "):
			add(rec[2:])
			continue
		case strings.HasPrefix(rec, "1 "):
			f = strings.SplitN(rec, " ", 9) // 1 XY sub mH mI mW hH hI path
		case strings.HasPrefix(rec, "u "):
			f = strings.SplitN(rec, " ", 11) // u XY sub m1 m2 m3 mW h1 h2 h3 path
		}
		if len(f) == 0 || f[len(f)-1] == "" || (f[0] == "1" && len(f) != 9) || (f[0] == "u" && len(f) != 11) {
			return "", nil, fmt.Errorf("unexpected git status entry %q", rec)
		}
		add(f[len(f)-1])
	}
	return oid, paths, nil
}

// gitErr is a failed git run's own explanation, when it gave one.
func gitErr(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(bytes.TrimSpace(ee.Stderr)) > 0 {
		return errors.New(string(bytes.TrimSpace(ee.Stderr)))
	}
	return err
}

// errNoCommits is a working-tree check in a repo whose HEAD has no commit yet:
// there is no base to compare against, and no root commit to identify taps by.
var errNoCommits = errors.New("no commits yet")

// aheadHint names the range that would check committed work, when HEAD is ahead
// of the remote's default branch or of its own upstream; "" when it is neither.
func (cs *changeset) aheadHint() string {
	var refs []string
	if def, err := cs.git("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		refs = append(refs, strings.TrimSpace(def))
	}
	if up, err := cs.git("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		refs = append(refs, strings.TrimSpace(up))
	}
	for _, ref := range refs {
		out, err := cs.git("rev-list", "--count", ref+"..HEAD")
		if n := strings.TrimSpace(out); err == nil && n != "0" {
			return fmt.Sprintf(" HEAD is %s commit(s) ahead of %s; `thresh check %s..HEAD` checks them.", n, ref, ref)
		}
	}
	return ""
}

func (cs *changeset) resolve(ref string) (string, error) {
	out, err := cs.git("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q to a commit", ref)
	}
	return strings.TrimSpace(out), nil
}

func (cs *changeset) git(args ...string) (string, error) {
	gitCalls++
	out, err := exec.Command("git", append([]string{"-C", cs.repo}, args...)...).Output()
	return string(out), err
}

// list is every file at ref, or in the working tree (tracked and untracked,
// ignored files aside) for ref "".
func (cs *changeset) list(ref string) []string {
	if l, ok := cs.lists[ref]; ok {
		return l
	}
	var out string
	if ref == "" {
		out, _ = cs.git("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	} else {
		out, _ = cs.git("ls-tree", "-r", "-z", "--name-only", ref)
	}
	var l []string
	seen := map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" && !seen[p] {
			seen[p] = true
			l = append(l, p)
		}
	}
	cs.lists[ref] = l
	return l
}

// blobs reads paths at ref through one git cat-file --batch, memoized. A path
// absent at ref reads as "".
func (cs *changeset) blobs(ref string, paths []string) map[string]string {
	out := map[string]string{}
	var need []string
	for _, p := range paths {
		if b, ok := cs.blob[ref+":"+p]; ok {
			out[p] = b
		} else {
			need = append(need, p)
		}
	}
	if len(need) == 0 {
		return out
	}
	var in strings.Builder
	for _, p := range need {
		in.WriteString(ref + ":" + p + "\n")
	}
	cmd := exec.Command("git", "-C", cs.repo, "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(in.String())
	gitCalls++
	raw, _ := cmd.Output()
	r := bufio.NewReader(bytes.NewReader(raw))
	for _, p := range need {
		content := ""
		// "<oid> <type> <size>" then the object and a newline; "<ref:path> missing"
		// (any other field count) has no body.
		if hdr, err := r.ReadString('\n'); err == nil {
			if f := strings.Fields(hdr); len(f) == 3 {
				if n, err := strconv.Atoi(f[2]); err == nil {
					buf := make([]byte, n+1)
					if _, err := io.ReadFull(r, buf); err == nil && f[1] == "blob" {
						content = string(buf[:n])
					}
				}
			}
		}
		cs.blob[ref+":"+p] = content
		out[p] = content
	}
	return out
}

// diff is every changed file's hunks, from one git diff of the changeset.
// Renames are not paired, so each file's hunks are those a diff of it alone shows.
func (cs *changeset) diff(path string) *fileDiff {
	if cs.diffs == nil {
		out, _ := cs.git("diff", "-U0", "--no-renames", "--no-color", "--no-ext-diff",
			"--src-prefix=a/", "--dst-prefix=b/", cs.diffspec)
		cs.diffs = parseDiff(out)
	}
	if d, ok := cs.diffs[path]; ok {
		return d
	}
	return &fileDiff{Added: map[int]bool{}}
}

func matchAny(globs []string, path string) bool {
	for _, g := range globs {
		if ok, _ := filepath.Match(g, path); ok {
			return true
		}
	}
	return false
}

// under returns the changed files matching any of the globs, fully loaded.
func (cs *changeset) under(globs []string) []changedFile {
	var out []changedFile
	for _, p := range cs.changed {
		if matchAny(globs, p) {
			out = append(out, cs.file(p))
		}
	}
	return out
}

// file loads one changed file's before/after/changed-lines (memoized).
func (cs *changeset) file(path string) changedFile {
	if f, ok := cs.cache[path]; ok {
		return *f
	}
	f := &changedFile{Path: path}
	f.Before = cs.blobs(cs.base, cs.changed)[path] // every changed file in one read
	if cs.worktree {
		b, _ := os.ReadFile(filepath.Join(cs.repo, path))
		f.After = string(b)
	} else {
		f.After = cs.blobs(cs.head, cs.changed)[path]
	}
	if f.Before == "" {
		f.Changed = allLines(f.After) // new file: every line is new
	} else {
		f.Changed = cs.diff(path).Added
	}
	cs.cache[path] = f
	return *f
}

// current returns the CURRENT content of every repo file matching the globs,
// changed or not. Needed by kinds that resolve references across files.
func (cs *changeset) current(globs []string) map[string]string {
	return cs.contentAt(globs, "")
}

// atBase returns the BASE content of every file matching the globs.
func (cs *changeset) atBase(globs []string) map[string]string {
	return cs.contentAt(globs, cs.base)
}

func (cs *changeset) contentAt(globs []string, ref string) map[string]string {
	disk := ref == "" && cs.worktree
	if ref == "" && !cs.worktree {
		ref = cs.head
	}
	var paths []string
	for _, p := range cs.list(ref) {
		if matchAny(globs, p) {
			paths = append(paths, p)
		}
	}
	if !disk {
		return cs.blobs(ref, paths)
	}
	out := map[string]string{}
	for _, p := range paths {
		b, _ := os.ReadFile(filepath.Join(cs.repo, p))
		out[p] = string(b)
	}
	return out
}

// touchedAny reports whether any changed file matches the globs.
func (cs *changeset) touchedAny(globs []string) bool {
	for _, p := range cs.changed {
		if matchAny(globs, p) {
			return true
		}
	}
	return false
}

func allLines(s string) map[int]bool {
	m := map[int]bool{}
	for i, n := 1, strings.Count(s, "\n")+1; i <= n; i++ {
		m[i] = true
	}
	return m
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
