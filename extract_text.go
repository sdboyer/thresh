package main

// Format-agnostic extractors: work on any text file.

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	lineCommentRE  = regexp.MustCompile(`//.*$`)
	blockCommentRE = regexp.MustCompile(`(?s)/\*.*?\*/`)
	hunkRE         = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
)

// stripNoise removes comments and normalizes whitespace so that two versions of
// a file compare equal iff their meaning-bearing content is the same. Covers the
// `//` and `/* */` syntaxes shared by Go and CUE. (A `//` inside a string literal
// is also stripped; acceptable for a did-meaning-change heuristic.)
func stripNoise(s string) string {
	s = blockCommentRE.ReplaceAllString(s, "")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = lineCommentRE.ReplaceAllString(l, "")
		l = strings.Join(strings.Fields(l), " ")
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// touched reports whether a file's meaning-bearing content changed. This is the
// shared precision lever: comment-only and whitespace-only edits are not touches,
// for every kind that uses it.
func touched(f changedFile) bool {
	return stripNoise(f.Before) != stripNoise(f.After)
}

// fileDiff is one file's hunks: the line numbers added or modified on the new
// side, and the content of every added and removed line.
type fileDiff struct {
	Added map[int]bool
	Lines []string
}

// parseDiff splits a zero-context unified diff of many files by file. A file's
// headers are read only before its first hunk, so a changed line that looks
// like a header ("--- x") is content.
func parseDiff(out string) map[string]*fileDiff {
	files := map[string]*fileDiff{}
	var cur *fileDiff
	var from string
	inHunk := false
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			cur, from, inHunk = nil, "", false
		case !inHunk && strings.HasPrefix(l, "--- "):
			from = diffPath(l[4:], "a/")
		case !inHunk && strings.HasPrefix(l, "+++ "):
			p := diffPath(l[4:], "b/")
			if p == "" {
				p = from // deleted
			}
			cur = &fileDiff{Added: map[int]bool{}}
			files[p] = cur
		case strings.HasPrefix(l, "@@"):
			inHunk = true
			m := hunkRE.FindStringSubmatch(l)
			if cur == nil || m == nil {
				continue
			}
			start, _ := strconv.Atoi(m[1])
			count := 1
			if m[2] != "" {
				count, _ = strconv.Atoi(m[2])
			}
			for i := 0; i < count; i++ {
				cur.Added[start+i] = true
			}
		case inHunk && cur != nil && l != "" && (l[0] == '+' || l[0] == '-'):
			cur.Lines = append(cur.Lines, l[1:])
		}
	}
	return files
}

// diffPath is the path a "--- "/"+++ " header names, without its prefix; "" for
// /dev/null. Git quotes unusual paths C-style and may end the name with a tab.
func diffPath(s, prefix string) string {
	s = strings.TrimRight(s, "\t")
	if strings.HasPrefix(s, `"`) {
		if u, err := strconv.Unquote(s); err == nil {
			s = u
		}
	}
	if s == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(s, prefix)
}

// changedLines returns the content of added and removed lines for a file.
func changedLines(cs *changeset, path string) []string {
	return cs.diff(path).Lines
}

// watched returns the changed lines containing any of the substrings.
func watched(lines, subs []string) []string {
	var hits []string
	seen := map[string]bool{}
	for _, l := range lines {
		for _, s := range subs {
			if strings.Contains(l, s) {
				t := strings.TrimSpace(l)
				if !seen[t] {
					seen[t] = true
					hits = append(hits, t)
				}
				break
			}
		}
	}
	return hits
}
