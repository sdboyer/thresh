package thresh

// The thresh framework's annotation contract. Embedded in the binary and
// unified against each repo's .thresh/ data at load — repos supply data that
// conforms to it, they do not restate it. (The tool's #Seam is a published
// language; each repo's .thresh is a conformist.)
//
// A seam is paths + kind. Each kind is a closed definition, so a seam's fields
// are exactly those its kind takes; typos and misplaced fields are rejected.

seams: [string]: #Seam

#Seam: #PublishedLanguage | #SharedKernel | #ChokePoint | #DeclaredSet | #ReferencesResolve

// A contract for consumers you cannot enumerate. You owe them compatibility, so
// the tap is a compatibility verdict from the format's own checker.
#PublishedLanguage: {
	kind:   "published-language"
	paths:  [...string]
	format: "thema" // compat extractor; other formats (protobuf, openapi, ...) are plugins to add
}

// Code co-owned by contexts you can name. You owe them consultation.
#SharedKernel: {
	kind:     "shared-kernel"
	paths:    [...string]
	consult?: [...string] // who stands on this; repeated in the tap
	watch?:   [...string] // if set, only changed lines containing one of these count
}

// All construction of `type` goes through `onlyIn`.
#ChokePoint: {
	kind:   "choke-point"
	paths:  [...string]
	type:   string
	onlyIn: [...string]
}

// An intentional, enumerated set of code elements. Undeclared additions and
// stale declarations are flagged; element bodies are not this kind's concern.
#DeclaredSet: {
	kind:     "declared-set"
	paths:    [...string]
	funcs:    string            // glob on function name, e.g. "check*"
	declared: [string]: string  // name -> why it belongs in the set
}

// Go reaches into CUE by literal path; fire when a reference no longer resolves.
#ReferencesResolve: {
	kind: "references-resolve"
	go:   [...string]
	cue:  [...string]
}
