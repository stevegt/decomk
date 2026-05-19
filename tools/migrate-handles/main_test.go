package main

import (
	"strings"
	"testing"
)

func TestDeterministicHandlesAreStableAndUnique(t *testing.T) {
	claimed := map[string]string{}
	first, err := mintDeterministic("TODO-husig", claimed)
	if err != nil {
		t.Fatalf("mint first: %v", err)
	}
	claimed[first] = "TODO-husig"
	second, err := mintDeterministic("TODO-husig", claimed)
	if err != nil {
		t.Fatalf("mint second: %v", err)
	}
	if first == second {
		t.Fatalf("collision retry returned same handle %q", first)
	}
	if !proquintHandleRE.MatchString(first) || !proquintHandleRE.MatchString(second) {
		t.Fatalf("handles are not proquints: %q %q", first, second)
	}
}

func TestRewriteOwnSubtasks(t *testing.T) {
	body := "- [ ] 025.1 first\n- [ ] 025.12 later\nrelated TODO 020.1 stays external\n"
	got := rewriteOwnSubtasks(body, "TODO-025", "kugod")
	if !strings.Contains(got, "kugod.1 first") || !strings.Contains(got, "kugod.12 later") {
		t.Fatalf("own subtasks not rewritten: %q", got)
	}
	if !strings.Contains(got, "TODO 020.1 stays external") {
		t.Fatalf("external reference changed unexpectedly: %q", got)
	}
}

func TestReplaceH1(t *testing.T) {
	got := replaceH1("# TODO 029 - Old\n\nbody\n", "TODO-vapoj", "New Title")
	if !strings.HasPrefix(got, "# TODO-vapoj: New Title") {
		t.Fatalf("unexpected H1: %q", got)
	}
}
