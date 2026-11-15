package main

// CUE extractors. Brace-scan and regex over formatted CUE, and a token scan for
// thema's version keys; these are the candidates to move onto the cue Go API if
// the tool ever goes in-process.

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
// version keys of every lineage in the file (see themaVersions). Other formats
// are plugins to add; an unknown format is a configuration error.
func versionTuples(src, format string) ([]ver, error) {
	switch format {
	case "thema":
		out := themaVersions(src)
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

// themaVersionKeyRE is thema's own rule for a major or minor version key.
var themaVersionKeyRE = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

// themaVersions reads the versions a thema lineage declares in keyed normal
// form: each pair of labels at the first two levels under a field named
// schemas, when both are version keys ("1": "0" is version 1.0). Chained labels
// ("1": "0": {...}, or schemas: "1": "0": {...}) and nested structs
// ("1": {"0": {...}}) spell the same key. Only labels count: comments, strings
// in values, and anything inside a schema are not versions. A version declared
// twice in one lineage counts once; each lineage in the file adds its own.
func themaVersions(src string) []ver {
	sc := &cueScanner{src: src}
	stack := []*cueFrame{{isStruct: true}}
	var out []ver
	seen := map[string]bool{}
	label, hasLabel := "", false // the label a following ':' completes
	ends := false                // a newline here ends the field (CUE's comma insertion)
	prev := cueTok{}
	lists := 0
	for {
		t := sc.next()
		f := stack[len(stack)-1]
		switch {
		case t.kind == 0:
			return out
		case t.kind == 'n':
			if ends && f.isStruct {
				f.chain = nil
				hasLabel = false
			}
			continue
		case t.kind == 's':
			label, hasLabel = t.text, true
		case t.kind == 'w':
			label, hasLabel = t.text, true
			if t.text[0] >= '0' && t.text[0] <= '9' {
				label = "" // a number; version keys are quoted
			}
		case t.text == "?" || t.text == "!" || t.text == "~":
			// optional and required markers, and the ~ of a postfix alias, keep the label
		case t.text == ":":
			if f.isStruct {
				if !hasLabel {
					label = ""
				}
				f.chain = append(f.chain, label)
				if in, root, rel := f.locate(); in && !f.opaque && len(rel) == 2 {
					key := root + "\x00" + rel[0] + "\x00" + rel[1]
					if !seen[key] && themaVersionKeyRE.MatchString(rel[0]) && themaVersionKeyRE.MatchString(rel[1]) {
						seen[key] = true
						out = append(out, ver{atoi(rel[0]), atoi(rel[1])})
					}
				}
			}
			hasLabel = false
		case t.text == ",":
			if f.isStruct {
				f.chain = nil
			}
			hasLabel = false
		case t.text == "{" || t.text == "[" || t.text == "(":
			c := &cueFrame{isStruct: t.text == "{"}
			if f.isStruct {
				c.abs = concat(f.abs, f.chain)
				c.inSchemas, c.root, c.rel = f.locate()
				c.opaque = f.opaque
			} else {
				lists++
				c.abs = concat(f.abs, []string{fmt.Sprintf("\x00%d", lists)})
				c.inSchemas, c.root = f.inSchemas, f.root
				c.opaque = f.inSchemas // nothing inside a list is a version key
			}
			if t.text == "(" && prev == (cueTok{kind: 'p', text: "~"}) {
				c.alias, c.saved = true, label
			}
			stack = append(stack, c)
			hasLabel = false
		case t.text == "}" || t.text == "]" || t.text == ")":
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			switch {
			case t.text == "}":
				hasLabel = false
			case f.alias:
				label, hasLabel = f.saved, true // "1"~(K,V): is labeled "1"
			default:
				label, hasLabel = "", true // a [pattern] or (dynamic) label is no version key
			}
		default:
			hasLabel = false
		}
		ends = t.kind == 'w' || t.kind == 's' || t.text == "}" || t.text == "]" || t.text == ")"
		prev = t
	}
}

// cueFrame is one open {struct}, [list] or (parenthesis) in a CUE token scan.
type cueFrame struct {
	isStruct  bool
	abs       []string // label path from the file root: the identity of a schemas struct
	chain     []string // labels of the field being read (a: b: c: is three)
	inSchemas bool     // inside a schemas struct
	root      string   // that schemas struct's identity
	rel       []string // label path from that schemas struct
	opaque    bool     // inside a list within schemas, where nothing is a version key
	alias     bool     // the (K,V) of a postfix alias; saved is the label it follows
	saved     string
}

// locate places the current field's labels relative to the enclosing schemas
// struct: inherited from the frame, or a schemas label in this field's chain.
func (f *cueFrame) locate() (in bool, root string, rel []string) {
	if f.inSchemas {
		return true, f.root, concat(f.rel, f.chain)
	}
	for i, l := range f.chain {
		if l == "schemas" {
			return true, strings.Join(concat(f.abs, f.chain[:i+1]), "\x00"), concat(nil, f.chain[i+1:])
		}
	}
	return false, "", nil
}

func concat(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}

// cueTok is a CUE token: 'w' a word (identifier, keyword, number), 's' a string
// (text is what lies between its quotes), 'n' a newline, 'p' punctuation (text
// is the character), 0 the end of input. Comments are skipped.
type cueTok struct {
	kind byte
	text string
}

type cueScanner struct {
	src string
	i   int
}

func (s *cueScanner) next() cueTok {
	for s.i < len(s.src) {
		c := s.src[s.i]
		switch {
		case c == '\n':
			s.i++
			return cueTok{kind: 'n'}
		case c == ' ' || c == '\t' || c == '\r':
			s.i++
		case strings.HasPrefix(s.src[s.i:], "//"):
			for s.i < len(s.src) && s.src[s.i] != '\n' {
				s.i++
			}
		case c == '"' || c == '\'' || c == '#' && rawStringAt(s.src[s.i:]):
			return s.str()
		case isCueWordByte(c):
			start := s.i
			for s.i < len(s.src) && isCueWordByte(s.src[s.i]) {
				s.i++
			}
			return cueTok{kind: 'w', text: s.src[start:s.i]}
		default:
			s.i++
			return cueTok{kind: 'p', text: string(c)}
		}
	}
	return cueTok{}
}

// rawStringAt reports whether src starts a #-delimited raw string.
func rawStringAt(src string) bool {
	rest := strings.TrimLeft(src, "#")
	return rest != "" && (rest[0] == '"' || rest[0] == '\'')
}

// str scans the string literal at s.i: double- or single-quoted, their
// triple-quoted multiline forms, and their #-delimited raw forms, skipping
// escapes and interpolations.
func (s *cueScanner) str() cueTok {
	hashes := 0
	for s.src[s.i] == '#' {
		hashes++
		s.i++
	}
	q := s.src[s.i : s.i+1]
	if strings.HasPrefix(s.src[s.i:], q+q+q) {
		q += q + q
	}
	s.i += len(q)
	end := q + strings.Repeat("#", hashes)
	esc := `\` + strings.Repeat("#", hashes)
	start := s.i
	for s.i < len(s.src) {
		switch {
		case strings.HasPrefix(s.src[s.i:], end):
			text := s.src[start:s.i]
			s.i += len(end)
			return cueTok{kind: 's', text: text}
		case len(q) == 1 && s.src[s.i] == '\n':
			return cueTok{kind: 's', text: s.src[start:s.i]} // unterminated
		case strings.HasPrefix(s.src[s.i:], esc+"("):
			s.i += len(esc) + 1
			for depth := 1; depth > 0; {
				switch t := s.next(); {
				case t.kind == 0:
					return cueTok{kind: 's', text: s.src[start:]}
				case t.kind == 'p' && t.text == "(":
					depth++
				case t.kind == 'p' && t.text == ")":
					depth--
				}
			}
		case strings.HasPrefix(s.src[s.i:], esc):
			s.i += len(esc) + 1
		default:
			s.i++
		}
	}
	return cueTok{kind: 's', text: s.src[start:]}
}

func isCueWordByte(c byte) bool {
	return c == '_' || c == '#' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
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

func fmtVer(v ver) string { return fmt.Sprintf("%d.%d", v[0], v[1]) }

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
