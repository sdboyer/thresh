package main

import "fmt"

// published-language: a contract for consumers you cannot enumerate. You owe them
// compatibility, so the tap is a compatibility verdict from the format's own
// checker. Fires on consequence: a correct breaking change still fires.
type publishedLanguage struct{}

func (publishedLanguage) assess(name string, s seam, cs *changeset) []signal {
	var out []signal
	for _, f := range cs.under(s.Paths) {
		if !touched(f) {
			continue
		}
		vb, err := versionTuples(f.Before, s.Format)
		if err != nil {
			return []signal{{Kind: s.Kind, Seam: name, File: f.Path, Level: "note", Message: err.Error()}}
		}
		va, _ := versionTuples(f.After, s.Format)
		if len(va) == 0 {
			continue
		}
		switch {
		case maxMajor(va) > maxMajor(vb):
			out = append(out, signal{Kind: s.Kind, Seam: name, File: f.Path, Level: "step-back",
				Message: fmt.Sprintf("breaking change: max version %s → %s (major bump). Consumers you don't control depend on this contract.",
					fmtVer(maxVer(vb)), fmtVer(maxVer(va)))})
		case !sameVersions(vb, va):
			out = append(out, signal{Kind: s.Kind, Seam: name, File: f.Path, Level: "note",
				Message: fmt.Sprintf("compatible change: versions %s → %s. The contract grew.", fmtVers(vb), fmtVers(va))})
		default:
			out = append(out, signal{Kind: s.Kind, Seam: name, File: f.Path, Level: "note",
				Message: "contract touched with no version change. Confirm its meaning is unchanged."})
		}
	}
	return out
}
