package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestSweepLiteralIDsAndPaths(t *testing.T) {
	rows := []row{{Kind: "TE", OldID: "TE-20260507-123000", NewID: "TE-vapoj", OldPath: "docs/thought-experiments/TE-20260507-123000-old.md", NewPath: "docs/thought-experiments/TE-vapoj-old.md"}}
	intReps, literalReps := buildReplacements(rows)
	got, edits := sweep("See TE-20260507-123000 and TE-20260507-123000-old.md", intReps, literalReps)
	if edits != 2 {
		t.Fatalf("edits = %d, want 2; got %q", edits, got)
	}
	if !strings.Contains(got, "TE-vapoj") || !strings.Contains(got, "TE-vapoj-old.md") {
		t.Fatalf("refs not swept: %q", got)
	}
}

func TestSweepTODOFormsWithSubtasks(t *testing.T) {
	rows := []row{{Kind: "TODO", OldID: "TODO-025", NewID: "TODO-kugod", OldPath: "TODO/025-old.md", NewPath: "TODO/TODO-kugod-old.md"}}
	intReps, literalReps := buildReplacements(rows)
	got, edits := sweep("TODO 025 and TODO-025.4 and TODO/025-old.md", intReps, literalReps)
	if edits != 3 {
		t.Fatalf("edits = %d, want 3; got %q", edits, got)
	}
	want := "TODO-kugod and TODO-kugod.4 and TODO/TODO-kugod-old.md"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNoOvermatchOnLongTODODate(t *testing.T) {
	intReps := []intRep{{re: regexp.MustCompile(`(^|[^\w-])TODO[\s/-]+0*25(\.[0-9]+)?($|[^\w-])`), to: "TODO-kugod"}}
	got, edits := sweep("TODO-20260422-2 is external", intReps, nil)
	if edits != 0 || got != "TODO-20260422-2 is external" {
		t.Fatalf("unexpected rewrite edits=%d got=%q", edits, got)
	}
}
