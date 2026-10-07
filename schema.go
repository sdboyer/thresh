package main

import _ "embed"

// seamSchema is the framework's annotation contract, embedded so it is the single
// source of truth. It is written into the isolated load dir and unified with each
// repo's .thresh/ data, which both validates the data and applies defaults.
// (Follows thema's embed.go idiom of co-versioning embedded CUE with the binary.)
//
//go:embed schema/seam.cue
var seamSchema string
