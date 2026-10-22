package main

// CUE extractors. Brace-scan and regex over formatted CUE today; these are the
// candidates to move onto the cue Go API if the tool ever goes in-process.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	cueDefRE    = regexp.MustCompile(`^(#[A-Za-z_][A-Za-z0-9_]*):`)
	cueLabelRE  = regexp.MustCompile(`^\s*(_?#?[A-Za-z_][A-Za-z0-9_]*)[?!]?:`)
	cueAnyDefRE = regexp.MustCompile(`^\s*(_?#[A-Za-z_][A-Za-z0-9_]*)[?!]?:`)
	themaVerRE  = regexp.MustCompile(`version:\s*\[\s*(\d+)\s*,\s*(\d+)\s*\]`)
)

type cueDef struct {
	Name       string
	Start, End int // 1-based line span
}

// cueDefinitions finds top-level #Definitions with their line spans.
func cueDefinitions(src string) []cueDef {
	lines := strings.Split(src, "\n")
	var out []cueDef
	for i := 0; i < len(lines); i++ {
		m := cueDefRE.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		d := cueDef{Name: m[1], Start: i + 1, End: i + 1}
		depth := strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
		for j := i + 1; depth > 0 && j < len(lines); j++ {
			depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
			d.End = j + 1
		}
		out = append(out, d)
	}
	return out
}

// cueDefLabel is a definition (#X) or hidden definition (_#X) declared at any
// depth, by the line its label starts.
type cueDefLabel struct {
	Name string
	Line int
}

// cueDefLabels finds every definition label in a CUE source, nested ones
// included: a definition inside a struct opens the same closedness question as
// a top-level one. A label must start its line, as formatted CUE has it.
func cueDefLabels(src string) []cueDefLabel {
	var out []cueDefLabel
	for i, l := range strings.Split(src, "\n") {
		if m := cueAnyDefRE.FindStringSubmatch(l); m != nil {
			out = append(out, cueDefLabel{Name: m[1], Line: i + 1})
		}
	}
	return out
}

// defsTouched names the top-level definitions whose span contains a changed line.
func defsTouched(f changedFile) []string {
	var names []string
	for _, d := range cueDefinitions(f.After) {
		for l := d.Start; l <= d.End; l++ {
			if f.Changed[l] {
				names = append(names, d.Name)
				break
			}
		}
	}
	return names
}

// cueLabels collects every field label, definition, and hidden definition across
// a set of CUE sources. Coarse (not scoped per definition) but exact for the
// question "does this name exist anywhere in the CUE?".
func cueLabels(srcs map[string]string) map[string]bool {
	labels := map[string]bool{}
	for _, src := range srcs {
		for _, l := range strings.Split(src, "\n") {
			if m := cueLabelRE.FindStringSubmatch(l); m != nil {
				labels[m[1]] = true
			}
		}
	}
	return labels
}

// --- published-language compat extractors, keyed by format ---------------------

type ver [2]int

// versionTuples extracts the declared versions of a contract. "thema" reads the
// `version: [major, minor]` tuples thema makes authors declare (and enforces).
// Other formats are plugins to add; an unknown format is a configuration error.
func versionTuples(src, format string) ([]ver, error) {
	switch format {
	case "thema":
		var out []ver
		for _, m := range themaVerRE.FindAllStringSubmatch(src, -1) {
			out = append(out, ver{atoi(m[1]), atoi(m[2])})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i][0] != out[j][0] {
				return out[i][0] < out[j][0]
			}
			return out[i][1] < out[j][1]
		})
		return out, nil
	}
	return nil, fmt.Errorf("published-language: unsupported format %q", format)
}

func maxMajor(vs []ver) int {
	m := -1
	for _, v := range vs {
		if v[0] > m {
			m = v[0]
		}
	}
	return m
}

func maxVer(vs []ver) ver {
	if len(vs) == 0 {
		return ver{-1, -1}
	}
	return vs[len(vs)-1]
}

func sameVersions(a, b []ver) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func fmtVer(v ver) string { return fmt.Sprintf("[%d,%d]", v[0], v[1]) }

func fmtVers(vs []ver) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmtVer(v)
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
