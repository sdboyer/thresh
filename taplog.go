package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

// --- The tap log ------------------------------------------------------------------
//
// A per-user, append-only record of every tap thresh emitted, which agent sessions
// were told about it, and whether it turned out to help. It is a separate judgment
// channel: nothing here ever feeds back into what check emits or how it exits.
// Every event carries a random eid and its host, so logs from many machines merge by
// concatenation and dedupe on eid.

type repoIdent struct {
	Root     string `json:"root"`               // root commit SHA: the identity
	Remote   string `json:"remote,omitempty"`   // normalized origin (host/owner/name): a label
	Toplevel string `json:"toplevel,omitempty"` // where it was checked out: provenance
}

// event is one line of the log. Ev says which fields apply.
type event struct {
	Ev   string    `json:"ev"` // sighting | delivered | verdict
	EID  string    `json:"eid"`
	At   time.Time `json:"at"`
	Host string    `json:"host,omitempty"`
	Tap  string    `json:"tap"`

	// sighting
	Repo     *repoIdent `json:"repo,omitempty"`
	Base     string     `json:"base,omitempty"`
	Head     string     `json:"head,omitempty"` // "" in working-tree mode
	Kind     string     `json:"kind,omitempty"`
	Seam     string     `json:"seam,omitempty"`
	File     string     `json:"file,omitempty"`
	Level    string     `json:"level,omitempty"`
	Message  string     `json:"message,omitempty"`
	SeamHash string     `json:"seamHash,omitempty"` // the seam's config, so a retuned seam is visible
	Thresh   string     `json:"thresh,omitempty"`   // the binary's vcs revision

	// sighting, delivered, verdict
	Session    string `json:"session,omitempty"`
	Transcript string `json:"transcript,omitempty"`

	// delivered
	MsgHash string `json:"msgHash,omitempty"`

	// verdict
	By       string `json:"by,omitempty"`     // origin | sweep | human
	Useful   string `json:"useful,omitempty"` // yes | no | unclear; "" when inaccurate
	Accurate *bool  `json:"accurate,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

func logPath() (string, error) {
	if p := os.Getenv("THRESH_LOG"); p != "" {
		return p, nil
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "thresh", "taps.jsonl"), nil
}

// appendEvents stamps and appends events, one O_APPEND write per line so
// concurrent sessions don't interleave within a line.
func appendEvents(path string, evs []event) error {
	if len(evs) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	host, _ := os.Hostname()
	now := time.Now().UTC()
	for _, e := range evs {
		e.EID = newEID()
		if e.At.IsZero() {
			e.At = now
		}
		e.Host = host
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func readEvents(path string) ([]event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for n := 1; sc.Scan(); n++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var e event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// --- Per-tap state, folded from the events ------------------------------------------

type sessionRef struct {
	Session    string `json:"session"`
	Transcript string `json:"transcript,omitempty"`
}

type tapState struct {
	ID        string       `json:"id"`
	Repo      repoIdent    `json:"repo"`
	Base      string       `json:"base"`
	Kind      string       `json:"kind"`
	Seam      string       `json:"seam"`
	File      string       `json:"file"`
	Level     string       `json:"level"`
	Message   string       `json:"message"` // latest
	FirstSeen time.Time    `json:"firstSeen"`
	LastSeen  time.Time    `json:"lastSeen"`
	Sightings int          `json:"sightings"`
	Sessions  []sessionRef `json:"sessions,omitempty"`

	delivered map[string]string // session -> msgHash most recently delivered
	verdicts  map[string]event  // recorder -> latest verdict
}

// fold reduces the log to per-tap state, in log order (latest wins).
func fold(evs []event) map[string]*tapState {
	taps := map[string]*tapState{}
	get := func(id string) *tapState {
		t, ok := taps[id]
		if !ok {
			t = &tapState{ID: id, delivered: map[string]string{}, verdicts: map[string]event{}}
			taps[id] = t
		}
		return t
	}
	for _, e := range evs {
		t := get(e.Tap)
		switch e.Ev {
		case "sighting":
			if e.Repo != nil {
				t.Repo = *e.Repo
			}
			t.Base, t.Kind, t.Seam, t.File, t.Level, t.Message = e.Base, e.Kind, e.Seam, e.File, e.Level, e.Message
			if t.FirstSeen.IsZero() || e.At.Before(t.FirstSeen) {
				t.FirstSeen = e.At
			}
			if e.At.After(t.LastSeen) {
				t.LastSeen = e.At
			}
			t.Sightings++
			if e.Session != "" && !hasSession(t.Sessions, e.Session) {
				t.Sessions = append(t.Sessions, sessionRef{e.Session, e.Transcript})
			}
		case "delivered":
			t.delivered[e.Session] = e.MsgHash
		case "verdict":
			t.verdicts[e.By] = e
		}
	}
	return taps
}

func hasSession(refs []sessionRef, s string) bool {
	for _, r := range refs {
		if r.Session == s {
			return true
		}
	}
	return false
}

// recorders in precedence order for the effective verdict.
var recorders = []string{"human", "origin", "sweep"}

func (t *tapState) effective() (event, bool) {
	for _, by := range recorders {
		if v, ok := t.verdicts[by]; ok {
			return v, true
		}
	}
	return event{}, false
}

// --- Identity -----------------------------------------------------------------------

// tapID is stable for one crossing of one seam by one change: the same tap
// re-sighted every turn keeps its ID, and its message may drift underneath it.
func tapID(root, base string, sig signal) string {
	return shortHash(strings.Join([]string{root, base, sig.Kind, sig.Seam, sig.File, sig.Level}, "\x00"))
}

func msgHash(msg string) string { return shortHash(msg) }

func seamHash(s seam) string {
	b, _ := json.Marshal(s) // map keys marshal sorted: deterministic
	return shortHash(string(b))
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

func newEID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// identify names the repo: its root commit (stable across clones, worktrees and
// machines) is the identity; the origin remote and checkout path are labels.
func (cs *changeset) identify() (repoIdent, error) {
	out, err := cs.git("rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return repoIdent{}, fmt.Errorf("finding root commit: %v", err)
	}
	roots := strings.Fields(out)
	if len(roots) == 0 {
		return repoIdent{}, fmt.Errorf("finding root commit: none")
	}
	sort.Strings(roots)
	id := repoIdent{Root: roots[0]}
	top, _ := cs.git("rev-parse", "--show-toplevel")
	id.Toplevel = strings.TrimSpace(top)
	remote, _ := cs.git("config", "--get", "remote.origin.url")
	id.Remote = normalizeRemote(remote)
	return id, nil
}

// normalizeRemote reduces the spellings of one remote (scp-like ssh, ssh://,
// https://, with or without user, port or .git) to host/owner/name.
func normalizeRemote(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	} else if i := strings.Index(u, ":"); i >= 0 && !strings.Contains(u[:i], "/") {
		u = u[:i] + "/" + u[i+1:] // scp-like host:path
	}
	if at, slash := strings.Index(u, "@"), strings.Index(u+"/", "/"); at >= 0 && at < slash {
		u = u[at+1:]
	}
	host, path, _ := strings.Cut(u, "/")
	host, _, _ = strings.Cut(host, ":")
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	return strings.ToLower(host + "/" + path)
}

func threshRevision() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "+dirty"
			}
		}
	}
	if rev == "" {
		return bi.Main.Version
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	return rev + dirty
}

// --- What check writes ----------------------------------------------------------------

func recordSightings(path string, cfg config, cs *changeset, id repoIdent, sigs []signal, session, transcript string) error {
	rev := threshRevision()
	var evs []event
	for _, s := range sigs {
		repo := id
		evs = append(evs, event{
			Ev: "sighting", Tap: s.ID, Repo: &repo, Base: cs.baseSHA, Head: cs.headSHA,
			Kind: s.Kind, Seam: s.Seam, File: s.File, Level: s.Level, Message: s.Message,
			SeamHash: seamHash(cfg.Seams[s.Seam]), Thresh: rev,
			Session: session, Transcript: transcript,
		})
	}
	return appendEvents(path, evs)
}

// undelivered returns the taps this session hasn't been told about at their
// current message, and records that it now has been. If the log can't be read,
// every tap is returned with the error: the judgment channel never suppresses one.
func undelivered(path, session string, sigs []signal) ([]signal, error) {
	evs, err := readEvents(path)
	if err != nil {
		return sigs, err
	}
	taps := fold(evs)
	var out []signal
	var marks []event
	for _, s := range sigs {
		h := msgHash(s.Message)
		if t, ok := taps[s.ID]; ok && t.delivered[session] == h {
			continue
		}
		out = append(out, s)
		marks = append(marks, event{Ev: "delivered", Tap: s.ID, Session: session, MsgHash: h})
	}
	return out, appendEvents(path, marks)
}
