package fixture

import "cuelang.org/go/cue"

// guarded may only be constructed through Build, the choke point.
type guarded struct{ v int }

func Build() *guarded { return &guarded{} }

// maybe carries the Go-side checks: an intentional, declared set.
type maybe struct{}

func (m *maybe) checkOne() error { return nil }
func (m *maybe) checkTwo() error { return nil }

// Literal references into the CUE kernel.
var (
	pathItems  = cue.MakePath(cue.Str("items"))
	pathKernel = cue.MakePath(cue.Def("#Kernel"))
)

func lookup() cue.Path { return cue.MakePath(cue.Str("lenses")) }
