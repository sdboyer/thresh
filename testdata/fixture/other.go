package fixture

import "cuelang.org/go/cue"

// Never touched by any scenario; it references a CUE label the binding depends
// on, so a rename in the CUE must be caught here too.
var pathLenses = cue.MakePath(cue.Str("lenses"))
