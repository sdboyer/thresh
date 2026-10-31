package main

// Python extractors, via the stdlib `ast` of the target's own python3 (extract_py.py, embedded). The
// target need not import or run. One run a distinct source; its facts are memoized.

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed extract_py.py
var pyScript string

type pyFunc struct {
	Name string `json:"name"` // qualified within the file: "Class.method", "outer.inner"
	Line int    `json:"line"`
}

type pyUse struct {
	Name string `json:"name"` // the dotted name referenced, its root resolved through the file's imports
	Line int    `json:"line"`
	Func string `json:"func"` // the qualified function it sits in; "" at module level
}

type pyEntry struct {
	Key  string `json:"key"`
	Line int    `json:"line"`
}

type pyFile struct {
	OK         bool                 `json:"ok"`
	Funcs      []pyFunc             `json:"funcs"`
	Uses       []pyUse              `json:"uses"`
	Registries map[string][]pyEntry `json:"registries"`
	Normalized string               `json:"normalized"` // the AST without docstrings
}

var (
	pyMemo     = map[[32]byte]*pyFile{}
	pyMemoLock sync.Mutex
	pyWarned   sync.Once
)

func isPython(path string) bool { return strings.HasSuffix(path, ".py") }

// pyFacts parses one Python source. It returns nil where python3 cannot be run (warned once on stderr,
// as every kind then stays silent on Python files) and a fact set with OK false where the source does
// not parse. $THRESH_PYTHON names the interpreter; python3 on PATH by default.
func pyFacts(src string) *pyFile {
	key := sha256.Sum256([]byte(src))
	pyMemoLock.Lock()
	defer pyMemoLock.Unlock()
	if f, ok := pyMemo[key]; ok {
		return f
	}
	python := os.Getenv("THRESH_PYTHON")
	if python == "" {
		python = "python3"
	}
	cmd := exec.Command(python, "-c", pyScript)
	cmd.Stdin = strings.NewReader(src)
	out, err := cmd.Output()
	var f pyFile
	if err == nil {
		err = json.Unmarshal(out, &f)
	}
	if err != nil {
		pyWarned.Do(func() {
			fmt.Fprintf(os.Stderr, "thresh: cannot run %s for Python files (%v); their seams stay silent\n", python, err)
		})
		return nil
	}
	pyMemo[key] = &f
	return &f
}

// pyUses lists the references to `name` or below it (`name.x`), with the function each sits in. `name`
// may be a glob ("a.b.tcgen05_*"), matched against each dotted prefix of a use.
func pyUses(src, name string) []pyUse {
	f := pyFacts(src)
	if f == nil || !f.OK {
		return nil
	}
	var out []pyUse
	for _, u := range f.Uses {
		parts := strings.Split(u.Name, ".")
		for i := range parts {
			prefix := strings.Join(parts[:i+1], ".")
			if ok, _ := filepath.Match(name, prefix); ok || prefix == name {
				out = append(out, u)
				break
			}
		}
	}
	return out
}

// pyFuncElements lists a Python source's functions and methods, by qualified name.
func pyFuncElements(src string) []element {
	var out []element
	if f := pyFacts(src); f != nil {
		for _, fn := range f.Funcs {
			out = append(out, element{fn.Name, fn.Line})
		}
	}
	return out
}

// pyRegistryElements lists the keys or elements of a module-level registry named `name`.
func pyRegistryElements(name string) func(string) []element {
	return func(src string) []element {
		var out []element
		if f := pyFacts(src); f != nil {
			for _, e := range f.Registries[name] {
				out = append(out, element{e.Key, e.Line})
			}
		}
		return out
	}
}

// pyTouched reports whether a Python file's meaning changed: its AST, without docstrings, differs.
// ok is false where either side cannot be parsed (the caller falls back to text).
func pyTouched(before, after string) (changed, ok bool) {
	b, a := pyFacts(before), pyFacts(after)
	if b == nil || a == nil || !b.OK || !a.OK {
		return false, false
	}
	return b.Normalized != a.Normalized, true
}
