package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// initRepo scaffolds .thresh/thresh.cue with every kind as a commented
// example. Onboarding a repo is: init, keep the seams that apply, then
// `thresh explain` to generate agent guidance.
func initRepo(repo string) error {
	dir := filepath.Join(repo, ".thresh")
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s already exists; not overwriting", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "thresh.cue")
	if err := os.WriteFile(path, []byte(initTemplate), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\nNext: uncomment and edit the seams that apply, then run 'thresh explain' to generate agent guidance for AGENTS.md.\n", path)
	return nil
}

const initTemplate = `package thresh

// Thresh annotations for this repo. Data only: the #Seam schema lives in the
// thresh binary and validates this file at load (no import needed).
//
// A seam is paths + kind. paths are plain globs; the kind decides what to look
// for. Every seam below is a commented example -- keep the ones that apply.
// Then run 'thresh explain' to render these as guidance for agents.

// published-language: a versioned contract for consumers you CANNOT enumerate.
// You owe them compatibility; a breaking change is loud even when correct.
// Needs a format with a compatibility checker (thema today).
// seams: "my-contract": {
// 	kind:   "published-language"
// 	paths:  ["schemas/*.cue"]
// 	format: "thema"
// }

// published-language over a golden corpus: the contract is recorded output, such
// as a checker's verdicts. section names the txtar section holding it (omit for
// the whole file); headline says its first line is the verdict, so a moved
// headline is loud and a reworded rest is a note.
// seams: "verdict-corpus": {
// 	kind:     "published-language"
// 	paths:    ["testdata/*.txtar"]
// 	format:   "golden"
// 	section:  "out"
// 	headline: true
// }

// shared-kernel: code co-owned by contexts you CAN name. You owe them consultation.
// seams: "core-types": {
// 	kind:    "shared-kernel"
// 	paths:   ["pkg/core/types.go"]
// 	consult: ["the API layer", "the storage layer"]
// }

// shared-kernel with watch: only changed lines mentioning these count.
// seams: "db-driver": {
// 	kind:    "shared-kernel"
// 	paths:   ["go.mod"]
// 	watch:   ["github.com/jackc/pgx"]
// 	consult: ["every query in pkg/storage"]
// }

// choke-point: all construction of a type goes through the named functions.
// seams: "connections": {
// 	kind:   "choke-point"
// 	paths:  ["*.go"]
// 	type:   "conn"
// 	onlyIn: ["Open"]
// }

// declared-set: an intentional, enumerated set of functions. An undeclared
// addition is flagged; a declared one that disappears is noted.
// seams: "validators": {
// 	kind:  "declared-set"
// 	paths: ["validate.go"]
// 	funcs: "check*"
// 	declared: {
// 		checkShape: "why this rule is enforced here and nowhere else"
// 	}
// }

// declared-set over CUE definitions (at any depth), instead of Go functions.
// seams: "kernel-defs": {
// 	kind:  "declared-set"
// 	paths: ["schema/*.cue"]
// 	defs:  "*#*" // hidden definitions are named _#X
// 	declared: {
// 		"#Root": "why this must be a definition, not a regular field"
// 	}
// }

// references-resolve: Go reaches into CUE by literal path (cue.Str/Def/Hid);
// every such reference must resolve in the CUE files.
// seams: "cue-go-binding": {
// 	kind: "references-resolve"
// 	go:   ["*.go"]
// 	cue:  ["*.cue"]
// }
`
