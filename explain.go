package main

import (
	"fmt"
	"strings"
)

// explain renders the repo's seams as guidance for an agent: what is watched and
// what a change there obliges. Generated from the annotations so it cannot drift
// from them; meant for an AGENTS.md section so the agent consults thresh
// mid-task, not only at the Stop hook.
func explain(cfg config) {
	fmt.Println("## Thresh seams")
	fmt.Println()
	fmt.Println("Before finalizing an approach that touches any of these, run `thresh check` and")
	fmt.Println("read the taps. They are facts about your diff, not opinions, and they never block.")
	fmt.Println()
	for _, name := range sortedKeys(cfg.Seams) {
		s := cfg.Seams[name]
		fmt.Printf("- **%s** (`%s`): %s\n", name, s.Kind, obligation(s))
	}
}

func obligation(s seam) string {
	p := strings.Join(s.Paths, ", ")
	switch s.Kind {
	case "published-language":
		return fmt.Sprintf("%s is a versioned contract for consumers you cannot enumerate. A change owes them compatibility; a breaking change is loud even when correct.", p)
	case "shared-kernel":
		who := "its co-owners"
		if len(s.Consult) > 0 {
			who = strings.Join(s.Consult, "; ")
		}
		if len(s.Watch) > 0 {
			return fmt.Sprintf("in %s, the lines mentioning %s are co-owned substrate. A change there owes consultation with: %s.", p, strings.Join(s.Watch, ", "), who)
		}
		return fmt.Sprintf("%s is co-owned kernel. A change there owes consultation with: %s.", p, who)
	case "choke-point":
		return fmt.Sprintf("in %s, only %s may construct %s. Constructing it elsewhere skips what the choke point enforces.", p, strings.Join(s.OnlyIn, "/"), s.Type)
	case "declared-set":
		return fmt.Sprintf("the %s functions in %s are an intentional, enumerated set (%d declared). Adding one requires declaring why it belongs.", s.Funcs, p, len(s.Declared))
	case "references-resolve":
		return fmt.Sprintf("Go files (%s) reach into CUE files (%s) by literal path; every such reference must resolve.", strings.Join(s.Go, ", "), strings.Join(s.Cue, ", "))
	}
	return "(unknown kind)"
}
