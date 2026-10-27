package thresh

// Annotations for the test fixture. Must stay in step with fixtureSeams in
// thresh_test.go (TestAnnotationsValidate asserts the seam names match).

seams: lineages: {
	kind:   "published-language"
	paths:  ["schemas/*.cue"]
	format: "thema"
}

seams: kernel: {
	kind:    "shared-kernel"
	paths:   ["kernel.cue"]
	consult: ["every consumer of #Kernel", "the Go binding"]
}

seams: "cue-module": {
	kind:  "shared-kernel"
	paths: ["cue.mod/module.cue"]
}

seams: "cue-dependency": {
	kind:  "shared-kernel"
	paths: ["go.mod"]
	watch: ["cuelang.org/go"]
}

seams: build: {
	kind:   "choke-point"
	paths:  ["*.go"]
	type:   "guarded"
	onlyIn: ["Build"]
}

seams: checks: {
	kind:  "declared-set"
	paths: ["bind.go"]
	funcs: "check*"
	declared: {
		checkOne: "first declared check"
		checkTwo: "second declared check"
	}
}

seams: shapes: {
	kind:  "declared-set"
	paths: ["shapes.cue"]
	defs:  "*#*"
	declared: {
		"#Shape": "closed shape"
		"#Edge":  "closed edge"
	}
}

seams: binding: {
	kind: "references-resolve"
	go:   ["*.go"]
	cue:  ["*.cue"]
}

seams: verdicts: {
	kind:     "published-language"
	paths:    ["corpus/*.txtar"]
	format:   "golden"
	section:  "out"
	headline: true
}
