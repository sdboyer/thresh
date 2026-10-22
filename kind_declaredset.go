package main

import (
	"fmt"
	"path/filepath"
)

// declared-set: a set of code elements that is intentional and enumerated. The
// human judges membership once, at declaration time (with a reason); the tool
// only enforces that the set never grows silently and never goes stale.
// Fires on wrongness. The *bodies* of declared elements are not its concern —
// whether they are correct belongs to the target's tests.
type declaredSet struct{}

func (declaredSet) assess(name string, s seam, cs *changeset) []signal {
	glob, elems := s.Funcs, goFuncElements
	if s.Defs != "" {
		glob, elems = s.Defs, cueDefElements
	}
	var out []signal
	for _, f := range cs.under(s.Paths) {
		before := map[string]bool{}
		for _, e := range elems(f.Before) {
			if ok, _ := filepath.Match(glob, e.name); ok {
				before[e.name] = true
			}
		}
		after := map[string]bool{}
		for _, e := range elems(f.After) {
			if ok, _ := filepath.Match(glob, e.name); !ok {
				continue
			}
			after[e.name] = true
			if _, declared := s.Declared[e.name]; !declared && !before[e.name] {
				out = append(out, signal{Kind: s.Kind, Seam: name, File: f.Path, Level: "step-back",
					Message: fmt.Sprintf("undeclared element added: %s (line %d). This set is intentional — declare why it belongs here, or it may not.",
						e.name, e.line)})
			}
		}
		for _, d := range sortedKeys(s.Declared) {
			if before[d] && !after[d] {
				out = append(out, signal{Kind: s.Kind, Seam: name, File: f.Path, Level: "note",
					Message: fmt.Sprintf("declared element %s no longer exists. Remove it from the declared set (did its rule move elsewhere?).", d)})
			}
		}
	}
	return out
}

type element struct {
	name string
	line int
}

func goFuncElements(src string) []element {
	var out []element
	for _, fn := range goFuncs(src) {
		out = append(out, element{fn.Name, fn.Line})
	}
	return out
}

func cueDefElements(src string) []element {
	var out []element
	for _, d := range cueDefLabels(src) {
		out = append(out, element{d.Name, d.Line})
	}
	return out
}
