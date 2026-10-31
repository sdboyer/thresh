package main

import (
	"fmt"
	"strings"
)

// choke-point: all construction of `type` goes through the functions in `onlyIn`.
// Reaching around the choke point skips whatever it enforces. Fires on wrongness,
// and only for sites on lines this changeset touched. In Go a site constructs the
// type; in Python it uses the dotted name `type` or anything under it (a call, an
// attribute, an import), through the file's imports. `onlyIn` entries are
// function names, globs allowed; a Python function is named as it is qualified
// in its file ("Profiler.start", "Profiler.*").
type chokePoint struct{}

func (chokePoint) assess(name string, s seam, cs *changeset) []signal {
	var out []signal
	for _, f := range cs.under(s.Paths) {
		if !touched(f) {
			continue
		}
		if isPython(f.Path) {
			for _, use := range pyUses(f.After, s.Type) {
				if !f.Changed[use.Line] || matchAny(s.OnlyIn, use.Func) {
					continue
				}
				where := use.Func + "()"
				if use.Func == "" {
					where = "module level"
				}
				out = append(out, signal{Kind: s.Kind, Seam: name, File: f.Path, Level: "step-back",
					Message: fmt.Sprintf("uses guarded name %s in %s (line %d) — outside the choke point (%s). Whatever it enforces is skipped on this path.",
						use.Name, where, use.Line, strings.Join(s.OnlyIn, ", "))})
			}
			continue
		}
		for _, site := range goConstructions(f.After, s.Type) {
			if !f.Changed[site.Line] || matchAny(s.OnlyIn, site.EnclosingFunc) {
				continue
			}
			out = append(out, signal{Kind: s.Kind, Seam: name, File: f.Path, Level: "step-back",
				Message: fmt.Sprintf("constructs guarded type %s in %s() (line %d) — outside the choke point (%s). Whatever it enforces is skipped on this path.",
					site.Type, site.EnclosingFunc, site.Line, strings.Join(s.OnlyIn, ", "))})
		}
	}
	return out
}
