// Command thresh taps the shoulder about consequential boundaries a changeset
// crossed, driven by per-repo annotations under .thresh/. It notifies; it never
// gates. Every signal is a fact about the diff.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

// options is every flag any command takes; each command reads the ones it needs.
type options struct {
	repo       string
	repoSet    bool
	pos        string // check: the range; verdict: the tap
	asJSON     bool
	session    string
	transcript string
	deliver    bool // check --new
	by         string
	useful     string
	evidence   string
	inaccurate bool
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		fmt.Print(usageText)
		return
	}
	o := options{repo: "."}
	for i, args := 0, os.Args[2:]; i < len(args); i++ {
		val := func() string {
			i++
			if i >= len(args) {
				usage()
			}
			return args[i]
		}
		switch args[i] {
		case "--repo":
			o.repo, o.repoSet = val(), true
		case "--json":
			o.asJSON = true
		case "--session":
			o.session = val()
		case "--transcript":
			o.transcript = val()
		case "--new":
			o.deliver = true
		case "--by":
			o.by = val()
		case "--useful":
			o.useful = val()
		case "--evidence":
			o.evidence = val()
		case "--inaccurate":
			o.inaccurate = true
		case "-h", "--help":
			fmt.Print(usageText)
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "thresh: unknown flag %s\n\n", args[i])
				usage()
			}
			o.pos = args[i]
		}
	}

	switch cmd {
	case "init":
		if err := initRepo(o.repo); err != nil {
			fail(err)
		}
		return
	case "version":
		fmt.Println("thresh", threshVersion())
		return
	case "verdict", "pending", "stats":
		if err := review(cmd, o, os.Stdout); err != nil {
			fail(err)
		}
		return
	case "check", "explain":
	default:
		usage()
	}

	cfg, err := loadConfig(o.repo)
	if err != nil {
		fail(err)
	}
	if len(cfg.Seams) == 0 {
		// Not a thresh repo, or nothing declared yet. Say so quietly and never
		// touch git, so a globally installed hook is silent and free everywhere else.
		switch {
		case cmd == "explain":
			fmt.Println("No seams declared. Run `thresh init` to scaffold .thresh/thresh.yaml.")
		case o.deliver:
		case o.asJSON:
			fmt.Println("[]")
		default:
			fmt.Println("Thresh: no seams declared (.thresh/ absent or empty).")
		}
		return
	}
	if cmd == "explain" {
		explain(os.Stdout, cfg)
		return
	}
	if err := runCheck(o, cfg, os.Stdout, os.Stderr); err != nil {
		fail(err)
	}
}

// runCheck assesses every seam, logs the taps, and reports them. Logging is a
// side channel: if it fails, that is a warning on stderr and the taps still print.
// With --new it reports only taps this session hasn't been told about yet, for
// delivery to the agent.
func runCheck(o options, cfg config, stdout, stderr io.Writer) error {
	if o.deliver && o.session == "" {
		return fmt.Errorf("--new needs --session")
	}
	cs, err := newChangeset(o.repo, o.pos)
	if errors.Is(err, errNoCommits) {
		switch {
		case o.asJSON:
			fmt.Fprintln(stdout, "[]")
		case o.deliver:
		default:
			fmt.Fprintln(stdout, "Thresh: this repo has no commits yet, so there is nothing to compare against. Commit once, then check.")
		}
		return nil
	}
	if err != nil {
		return err
	}
	var signals []signal
	for _, name := range sortedKeys(cfg.Seams) { // seams evaluate independently
		s := cfg.Seams[name]
		k, ok := kinds[s.Kind]
		if !ok {
			return fmt.Errorf("seam %q: unknown kind %q", name, s.Kind)
		}
		signals = append(signals, k.assess(name, s, cs)...)
	}

	// Taps are identified and logged; with none, there is nothing to identify,
	// record or deliver, so neither the repo's identity nor the log is read.
	if len(signals) > 0 {
		id, err := cs.identify()
		if err != nil {
			return err
		}
		for i := range signals {
			signals[i].ID = tapID(id.Root, cs.baseSHA, signals[i])
		}
		path, err := logPath()
		if err == nil {
			err = recordSightings(path, cfg, cs, id, signals, o.session, o.transcript)
		}
		if err != nil {
			fmt.Fprintf(stderr, "thresh: tap log not written: %v\n", err)
		}
		if o.deliver {
			if err == nil {
				signals, err = undelivered(path, o.session, signals)
			}
			if err != nil {
				fmt.Fprintf(stderr, "thresh: tap log unreadable, delivering every tap: %v\n", err)
			}
		}
	}

	switch {
	case o.asJSON:
		if signals == nil {
			signals = []signal{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(signals)
	case o.deliver:
		if len(signals) > 0 {
			render(stdout, signals)
			fmt.Fprintln(stdout)
			fmt.Fprint(stdout, protocol)
		}
	case cs.worktree && len(cs.changed) == 0:
		fmt.Fprintln(stdout, "Thresh: the working tree has no uncommitted changes, so there is nothing to check."+cs.aheadHint())
	default:
		render(stdout, signals)
	}
	return nil
}

// review runs the commands over the tap log.
func review(cmd string, o options, stdout io.Writer) error {
	path, err := logPath()
	if err != nil {
		return err
	}
	if cmd == "verdict" {
		id, err := recordVerdict(path, verdictArgs{tap: o.pos, by: o.by, useful: o.useful,
			evidence: o.evidence, session: o.session, inaccurate: o.inaccurate})
		if err == nil {
			fmt.Fprintf(stdout, "recorded verdict on #%s\n", id)
		}
		return err
	}
	evs, err := readEvents(path)
	if err != nil {
		return err
	}
	taps := fold(evs)
	if cmd == "stats" {
		renderStats(stdout, computeStats(taps), o.asJSON)
		return nil
	}
	root := ""
	if o.repoSet {
		cs := &changeset{repo: o.repo}
		id, err := cs.identify()
		if err != nil {
			return err
		}
		root = id.Root
	}
	renderPending(stdout, pending(taps, root), o.asJSON)
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, usageText)
	os.Exit(2)
}

const usageText = `usage:
  thresh check [A..B] [--repo DIR] [--json]   tap the shoulder about boundaries the changeset crossed
                                                 (no range => uncommitted working tree vs HEAD;
                                                 A..B => B's commits since it left A, from their merge base)
  thresh explain [--repo DIR]                 render the seams as agent guidance (for AGENTS.md)
  thresh init [--repo DIR]                    scaffold .thresh/thresh.yaml with every kind as an example
  thresh version                              the release this binary is, and the commit it was built from

  check also takes --session ID --transcript PATH (provenance for the tap log) and
  --new (report only taps this session hasn't been told about; needs --session).

tap log ($THRESH_LOG, default $XDG_STATE_HOME/thresh/taps.jsonl):
  thresh verdict ID --by origin|sweep|human (--useful yes|no|unclear | --inaccurate) --evidence TEXT
                                              record whether a tap helped
  thresh pending [--repo DIR] [--json]        taps nobody has given a verdict on
  thresh stats [--json]                       taps and verdicts by kind, repo and seam

  -h, --help on any command prints this.
`

// threshVersion is the module version the binary was built as (a release tag, a
// pseudo-version, or "(devel)"), with its commit when the version doesn't already name it.
func threshVersion() string {
	var v string
	if bi, ok := debug.ReadBuildInfo(); ok {
		v = bi.Main.Version
	}
	return versionOf(v, threshRevision())
}

func versionOf(version, rev string) string {
	switch {
	case version == "":
		return rev
	case rev == "" || strings.Contains(version, strings.TrimSuffix(rev, "+dirty")):
		return version
	}
	return version + " (" + rev + ")"
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "thresh: %v\n", err)
	os.Exit(1)
}

func render(w io.Writer, signals []signal) {
	if len(signals) == 0 {
		fmt.Fprintln(w, "Thresh: no consequential boundaries touched.")
		return
	}
	fmt.Fprintln(w, "## Thresh")
	for _, s := range signals {
		marker := "·"
		if s.Level == "step-back" {
			marker = "⚑ STEP BACK"
		}
		where := s.File
		if where == "" {
			where = "(seam-wide)"
		}
		fmt.Fprintf(w, "%s [%s: %s] %s  #%s\n    %s\n", marker, s.Kind, s.Seam, where, s.ID, s.Message)
	}
}
