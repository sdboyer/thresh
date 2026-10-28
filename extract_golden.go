package main

// Golden-output extractors: the recorded output a corpus pins, read from a whole
// file or from one section of a txtar archive.

import "strings"

// recordedOutput returns the output a golden file records: the named txtar
// section, or the whole file when section is "". ok is false when the section
// is absent.
func recordedOutput(src, section string) (out string, ok bool) {
	if section == "" {
		return src, true
	}
	return txtarSection(src, section)
}

// txtarSection returns the content of the txtar file named name: the lines
// after its `-- name --` marker, up to the next marker or the end.
func txtarSection(src, name string) (string, bool) {
	var b strings.Builder
	in, found := false, false
	for _, l := range strings.SplitAfter(src, "\n") {
		if n, isMarker := txtarMarker(l); isMarker {
			if in {
				break
			}
			in = n == name
			found = found || in
			continue
		}
		if in {
			b.WriteString(l)
		}
	}
	return b.String(), found
}

// txtarMarker reports whether a line is a txtar file marker, `-- name --`.
func txtarMarker(l string) (string, bool) {
	l = strings.TrimRight(l, "\r\n")
	if !strings.HasPrefix(l, "-- ") || !strings.HasSuffix(l, " --") || len(l) < 7 {
		return "", false
	}
	return strings.TrimSpace(l[3 : len(l)-3]), true
}

// headline is the first line of a recorded output.
func headline(out string) string {
	first, _, _ := strings.Cut(out, "\n")
	return strings.TrimSpace(first)
}
