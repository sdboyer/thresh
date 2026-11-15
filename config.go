package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// --- The annotation contract ---------------------------------------------------
//
// A seam is purpose + paths + kind. kindFields is the single source for which
// fields each kind takes: every kind is closed, so a typo or a field that belongs
// to another kind is rejected at load, as is a missing required one.

type field struct {
	name     string
	required bool
}

var kindFields = map[string][]field{
	// A contract for consumers you cannot enumerate. You owe them compatibility,
	// so the tap is a compatibility verdict read from the format.
	"published-language": {{"paths", true}, {"format", true}, {"section", false}, {"headline", false}},
	// Code co-owned by contexts you can name. You owe them consultation.
	"shared-kernel": {{"paths", true}, {"consult", false}, {"watch", false}},
	// All construction of type goes through onlyIn.
	"choke-point": {{"paths", true}, {"type", true}, {"onlyIn", true}},
	// An intentional, enumerated set of Go functions (funcs) or CUE definitions
	// (defs): exactly one of the two.
	"declared-set": {{"paths", true}, {"funcs", false}, {"defs", false}, {"declared", false}},
	// Go reaches into CUE by literal path; every reference must resolve.
	"references-resolve": {{"go", true}, {"cue", true}},
}

// Every kind takes these.
var commonFields = []field{{"kind", true}, {"purpose", true}}

// formats are the published-language formats with a compat extractor.
var formats = []string{"thema", "golden"}

// loadConfig reads every .thresh/*.yaml (and *.yml) and validates each seam
// against kindFields. Files compose by seam name; a name declared twice is an
// error, never a merge. Every problem is reported, each with its file and line.
func loadConfig(repo string) (config, error) {
	dir := filepath.Join(repo, ".thresh")
	if cues, _ := filepath.Glob(filepath.Join(dir, "*.cue")); len(cues) > 0 {
		return config{}, fmt.Errorf("%s: thresh config is YAML now; move these seams into .thresh/thresh.yaml (thresh init in an empty directory shows the shape)", strings.Join(relAll(repo, cues), ", "))
	}
	yamls, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	ymls, _ := filepath.Glob(filepath.Join(dir, "*.yml"))
	files := append(yamls, ymls...)
	sort.Strings(files)
	if len(files) == 0 {
		return config{}, nil // no annotations: nothing consequential is declared
	}

	cfg := config{Seams: map[string]seam{}}
	declaredAt := map[string]string{}
	var problems []string
	for _, f := range files {
		rel, _ := filepath.Rel(repo, f)
		b, err := os.ReadFile(f)
		if err != nil {
			return config{}, err
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(b, &doc); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", rel, err))
			continue
		}
		if len(doc.Content) == 0 {
			continue // empty, or only comments
		}
		top := doc.Content[0]
		if top.Kind != yaml.MappingNode {
			problems = append(problems, fmt.Sprintf("%s:%d: expected a mapping with a seams key", rel, top.Line))
			continue
		}
		for i := 0; i < len(top.Content); i += 2 {
			k, v := top.Content[i], top.Content[i+1]
			if k.Value != "seams" {
				problems = append(problems, fmt.Sprintf("%s:%d: unknown top-level key %q; the only one is seams", rel, k.Line, k.Value))
				continue
			}
			if v.Tag == "!!null" {
				continue // seams: with every example still commented out
			}
			if v.Kind != yaml.MappingNode {
				problems = append(problems, fmt.Sprintf("%s:%d: seams must be a mapping of seam name to seam", rel, v.Line))
				continue
			}
			for j := 0; j < len(v.Content); j += 2 {
				nk, nv := v.Content[j], v.Content[j+1]
				name := nk.Value
				at := fmt.Sprintf("%s:%d", rel, nk.Line)
				if prev, dup := declaredAt[name]; dup {
					problems = append(problems, fmt.Sprintf("%s: seam %q is already declared at %s", at, name, prev))
					continue
				}
				declaredAt[name] = at
				s, errs := decodeSeam(nv)
				sort.SliceStable(errs, func(a, b int) bool { return errs[a].line < errs[b].line })
				for _, e := range errs {
					problems = append(problems, fmt.Sprintf("%s:%d: seam %q: %s", rel, e.line, name, e.msg))
				}
				if len(errs) == 0 {
					cfg.Seams[name] = s
				}
			}
		}
	}
	if len(problems) > 0 {
		return config{}, fmt.Errorf("invalid .thresh annotations:\n%s", strings.Join(problems, "\n"))
	}
	return cfg, nil
}

type seamError struct {
	line int
	msg  string
}

// decodeSeam checks one seam's keys against its kind's closed field set, then
// decodes it and checks its values, reporting every problem it finds. A kind that
// isn't known stops the checks there: its field set is unknown, so anything said
// about its fields would be noise.
func decodeSeam(n *yaml.Node) (seam, []seamError) {
	if n.Kind != yaml.MappingNode {
		return seam{}, []seamError{{n.Line, "expected a mapping of fields"}}
	}
	keys := map[string]*yaml.Node{}
	vals := map[string]*yaml.Node{}
	for i := 0; i < len(n.Content); i += 2 {
		keys[n.Content[i].Value] = n.Content[i]
		vals[n.Content[i].Value] = n.Content[i+1]
	}

	kindNames := sortedKeys(kindFields)
	kv, ok := vals["kind"]
	if !ok {
		return seam{}, []seamError{{n.Line, fmt.Sprintf("kind is required, one of: %s", strings.Join(kindNames, ", "))}}
	}
	fields, ok := kindFields[kv.Value]
	if !ok {
		return seam{}, []seamError{{kv.Line, fmt.Sprintf("unknown kind %q; kinds are: %s", kv.Value, strings.Join(kindNames, ", "))}}
	}
	fields = append(append([]field{}, commonFields...), fields...)

	var errs []seamError
	allowed := map[string]bool{}
	var names []string
	for _, f := range fields {
		allowed[f.name] = true
		names = append(names, f.name)
		if v, ok := vals[f.name]; f.required && (!ok || v.Tag == "!!null") {
			errs = append(errs, seamError{n.Line, fmt.Sprintf("%s is required for kind %s", f.name, kv.Value)})
		}
	}
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		if !allowed[k.Value] {
			errs = append(errs, seamError{k.Line, fmt.Sprintf("field %q is not part of kind %s, which takes: %s", k.Value, kv.Value, strings.Join(names, ", "))})
		}
	}
	var s seam
	if err := n.Decode(&s); err != nil {
		te, ok := err.(*yaml.TypeError)
		if !ok {
			return seam{}, append(errs, seamError{n.Line, err.Error()})
		}
		for _, e := range te.Errors {
			line := n.Line
			if _, err := fmt.Sscanf(e, "line %d: ", &line); err == nil {
				e = e[strings.Index(e, ": ")+2:]
			}
			errs = append(errs, seamError{line, e})
		}
		return seam{}, errs
	}
	if k, ok := keys["purpose"]; ok && vals["purpose"].Tag != "!!null" && strings.TrimSpace(s.Purpose) == "" {
		errs = append(errs, seamError{k.Line, "purpose must say why the seam exists; it can't be empty"})
	}
	switch s.Kind {
	case "published-language":
		if v, ok := vals["format"]; ok && !slices.Contains(formats, s.Format) {
			errs = append(errs, seamError{v.Line, fmt.Sprintf("unknown format %q; formats are: %s", s.Format, strings.Join(formats, ", "))})
		}
		for _, f := range []string{"section", "headline"} {
			if k, ok := keys[f]; ok && s.Format != "golden" {
				errs = append(errs, seamError{k.Line, fmt.Sprintf("%s applies only to format golden", f)})
			}
		}
	case "declared-set":
		if (s.Funcs == "") == (s.Defs == "") {
			errs = append(errs, seamError{n.Line, "declared-set takes exactly one of funcs (Go functions) or defs (CUE definitions)"})
		}
	}
	return s, errs
}

func relAll(base string, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i], _ = filepath.Rel(base, p)
	}
	return out
}
