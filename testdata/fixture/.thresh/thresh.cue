package thresh

// Annotations for the test fixture. Must stay in step with fixtureSeams in
// thresh_test.go (TestAnnotationsValidate asserts the seam names match).

seams: lineages: {
	kind:    "published-language"
	purpose: "consumers outside this repo bind to the contract's versions"
	paths:   ["schemas/*.cue"]
	format:  "thema"
}

seams: kernel: {
	kind:    "shared-kernel"
	purpose: "#Kernel is what both the CUE and Go sides stand on"
	paths:   ["kernel.cue"]
	consult: ["every consumer of #Kernel", "the Go binding"]
}

seams: "cue-module": {
	kind:    "shared-kernel"
	purpose: "the module config decides how all the CUE evaluates"
	paths:   ["cue.mod/module.cue"]
}

seams: "cue-dependency": {
	kind:    "shared-kernel"
	purpose: "the CUE implementation is the language everything here is written in"
	paths:   ["go.mod"]
	watch:   ["cuelang.org/go"]
}

seams: build: {
	kind:    "choke-point"
	purpose: "Build is where a guarded value's invariants are established"
	paths:   ["*.go"]
	type:    "guarded"
	onlyIn:  ["Build"]
}

seams: checks: {
	kind:    "declared-set"
	purpose: "each check is a rule CUE cannot express, and says why"
	paths:   ["bind.go"]
	funcs:   "check*"
	declared: {
		checkOne: "first declared check"
		checkTwo: "second declared check"
	}
}

seams: shapes: {
	kind:    "declared-set"
	purpose: "every definition closes what flows through it, so each one is deliberate"
	paths:   ["shapes.cue"]
	defs:    "*#*"
	declared: {
		"#Shape": "closed shape"
		"#Edge":  "closed edge"
	}
}

seams: binding: {
	kind:    "references-resolve"
	purpose: "the Go binding reads the CUE by literal path"
	go:      ["*.go"]
	cue:     ["*.cue"]
}
