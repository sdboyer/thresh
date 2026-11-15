package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// initRepo scaffolds .thresh/thresh.yaml with every kind as a commented
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
	path := filepath.Join(dir, "thresh.yaml")
	if err := os.WriteFile(path, []byte(initTemplate), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\nNext: uncomment and edit the seams that apply, then run 'thresh explain' to generate agent guidance for AGENTS.md.\n", path)
	return nil
}

const initTemplate = `# Thresh annotations for this repo. thresh validates this file at load: each
# kind takes a fixed set of fields, so a typo or a misplaced field is an error.
#
# A seam is a purpose, paths and a kind. purpose says why the seam exists, in
# your words: it is required, and thresh explain renders it, so it belongs here
# rather than in AGENTS.md prose. paths are plain globs; the kind decides what
# to look for. Every seam below is a commented example: uncomment the ones that
# apply (every line starting "  # ") and edit them. Then run 'thresh explain' to
# render them as guidance for agents.
#
# Quote any value starting with * or # ("*.go", "#Root"): in YAML those
# start an alias and a comment.
seams:
  # my-contract:
  #   # published-language: a versioned contract for consumers you CANNOT
  #   # enumerate. You owe them compatibility; a breaking change is loud even
  #   # when correct. Needs a format with a compatibility checker.
  #   kind: published-language
  #   purpose: clients we don't ship decode these; a breaking change strands them
  #   paths: ["schemas/*.cue"]
  #   format: thema

  # verdict-corpus:
  #   # published-language over a golden corpus: the contract is recorded
  #   # output, such as a checker's verdicts. section names the txtar section
  #   # holding it (omit for the whole file); headline says its first line is
  #   # the verdict, so a moved headline is loud and a reworded rest is a note.
  #   kind: published-language
  #   purpose: tools downstream match on these verdicts; a moved one misleads them
  #   paths: ["testdata/*.txtar"]
  #   format: golden
  #   section: out
  #   headline: true

  # core-types:
  #   # shared-kernel: code co-owned by contexts you CAN name. You owe them
  #   # consultation.
  #   kind: shared-kernel
  #   purpose: the API and storage layers both build on these types
  #   paths: [pkg/core/types.go]
  #   consult: [the API layer, the storage layer]

  # db-driver:
  #   # shared-kernel with watch: only changed lines mentioning these count.
  #   kind: shared-kernel
  #   purpose: every query is written against this driver's behavior
  #   paths: [go.mod]
  #   watch: [github.com/jackc/pgx]
  #   consult: [every query in pkg/storage]

  # connections:
  #   # choke-point: all construction of a type goes through the named functions.
  #   kind: choke-point
  #   purpose: Open sets the timeouts and TLS config every connection needs
  #   paths: ["*.go"]
  #   type: conn
  #   onlyIn: [Open]

  # validators:
  #   # declared-set: an intentional, enumerated set of functions. An undeclared
  #   # addition is flagged; a declared one that disappears is noted.
  #   kind: declared-set
  #   purpose: validation belongs in the schema; each Go check is an exception
  #   paths: [validate.go]
  #   funcs: "check*"
  #   declared:
  #     checkShape: why this rule is enforced here and nowhere else

  # kernel-defs:
  #   # declared-set over CUE definitions (at any depth), instead of Go
  #   # functions. Hidden definitions are named _#X, so "*#*" covers both.
  #   kind: declared-set
  #   purpose: definitions close everything that flows through them
  #   paths: [schema/*.cue]
  #   defs: "*#*"
  #   declared:
  #     "#Root": why this must be a definition, not a regular field

  # cue-go-binding:
  #   # references-resolve: Go reaches into CUE by literal path
  #   # (cue.Str/Def/Hid); every such reference must resolve in the CUE files.
  #   kind: references-resolve
  #   purpose: the Go side reads the CUE by literal path
  #   go: ["*.go"]
  #   cue: ["*.cue"]
`
