package main

import (
	"fmt"
	"strings"
)

// choke-point: all construction of `type` goes through the functions in `onlyIn`.
// Reaching around the choke point skips whatever it enforces. Fires on wrongness,
// and only for constructions on lines this changeset touched.
type chokePoint struct{}

func (chokePoint) assess(name string, s seam, cs *changeset) []signal {
	allowed := map[string]bool{}
	for _, fn := range s.OnlyIn {
		allowed[fn] = true
	}
	var out []signal
	for _, f := range cs.under(s.Paths) {
		if !touched(f) {
			continue
		}
		for _, site := range goConstructions(f.After, s.Type) {
			if !f.Changed[site.Line] || allowed[site.EnclosingFunc] {
				continue
			}
			out = append(out, signal{Kind: s.Kind, Seam: name, File: f.Path, Level: "step-back",
				Message: fmt.Sprintf("constructs guarded type %s in %s() (line %d) — outside the choke point (%s). Whatever it enforces is skipped on this path.",
					site.Type, site.EnclosingFunc, site.Line, strings.Join(s.OnlyIn, ", "))})
		}
	}
	return out
}
