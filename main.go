// Command thresh taps the shoulder about consequential boundaries a changeset
// crossed, driven by per-repo annotations under .thresh/. It notifies; it never
// gates. Every signal is a fact about the diff.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	repo, rng, asJSON := ".", "", false
	for i, args := 0, os.Args[2:]; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			i++
			repo = args[i]
		case "--json":
			asJSON = true
		default:
			rng = args[i]
		}
	}

	switch cmd {
	case "init":
		if err := initRepo(repo); err != nil {
			fail(err)
		}
		return
	case "check", "explain":
	default:
		usage()
	}

	cfg, err := loadConfig(repo)
	if err != nil {
		fail(err)
	}
	if len(cfg.Seams) == 0 {
		// Not a thresh repo, or nothing declared yet. Say so quietly and never
		// touch git, so a globally installed hook is silent and free everywhere else.
		switch {
		case cmd == "explain":
			fmt.Println("No seams declared. Run `thresh init` to scaffold .thresh/thresh.cue.")
		case asJSON:
			fmt.Println("[]")
		default:
			fmt.Println("Thresh: no seams declared (.thresh/ absent or empty).")
		}
		return
	}
	if cmd == "explain" {
		explain(cfg)
		return
	}
	cs, err := newChangeset(repo, rng)
	if err != nil {
		fail(err)
	}

	var signals []signal
	for _, name := range sortedKeys(cfg.Seams) { // seams evaluate independently
		s := cfg.Seams[name]
		k, ok := kinds[s.Kind]
		if !ok {
			fail(fmt.Errorf("seam %q: unknown kind %q", name, s.Kind))
		}
		signals = append(signals, k.assess(name, s, cs)...)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(signals)
		return
	}
	render(signals)
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  thresh check [A..B] [--repo DIR] [--json]   tap the shoulder about boundaries the changeset crossed
                                                 (no range => uncommitted working tree vs HEAD)
  thresh explain [--repo DIR]                 render the seams as agent guidance (for AGENTS.md)
  thresh init [--repo DIR]                    scaffold .thresh/thresh.cue with every kind as an example
`)
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "thresh: %v\n", err)
	os.Exit(1)
}

func render(signals []signal) {
	if len(signals) == 0 {
		fmt.Println("Thresh: no consequential boundaries touched.")
		return
	}
	fmt.Println("## Thresh")
	for _, s := range signals {
		marker := "·"
		if s.Level == "step-back" {
			marker = "⚑ STEP BACK"
		}
		where := s.File
		if where == "" {
			where = "(seam-wide)"
		}
		fmt.Printf("%s [%s: %s] %s\n    %s\n", marker, s.Kind, s.Seam, where, s.Message)
	}
}

// loadConfig exports the repo's .thresh/*.cue in isolation (they are
// thresh's config, not part of the target's CUE module) unified with the
// embedded #Seam schema, which validates the data and single-sources the contract.
func loadConfig(repo string) (config, error) {
	files, _ := filepath.Glob(filepath.Join(repo, ".thresh", "*.cue"))
	if len(files) == 0 {
		return config{}, nil // no annotations: nothing consequential is declared
	}
	tmp, err := os.MkdirTemp("", "thresh-cfg-")
	if err != nil {
		return config{}, err
	}
	defer os.RemoveAll(tmp)
	if err := os.WriteFile(filepath.Join(tmp, "zz_thresh_schema.cue"), []byte(seamSchema), 0o644); err != nil {
		return config{}, err
	}
	names := []string{"zz_thresh_schema.cue"}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return config{}, err
		}
		if err := os.WriteFile(filepath.Join(tmp, filepath.Base(f)), b, 0o644); err != nil {
			return config{}, err
		}
		names = append(names, filepath.Base(f))
	}
	cmd := exec.Command("cue", append([]string{"export", "--out", "json"}, names...)...)
	cmd.Dir = tmp
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return config{}, fmt.Errorf("invalid .thresh annotations:\n%s", stderr)
	}
	var cfg config
	if err := json.Unmarshal(out, &cfg); err != nil {
		return config{}, fmt.Errorf("parse annotations: %v", err)
	}
	return cfg, nil
}
