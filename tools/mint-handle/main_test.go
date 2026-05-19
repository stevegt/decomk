package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMintWidthOne(t *testing.T) {
	got, err := mint(1, map[string]string{}, 0, false)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("len(%q) = %d, want 5", got, len(got))
	}
	if !handleFileRE.MatchString("TE-" + got + "-x.md") {
		t.Fatalf("mint %q does not match handle regex", got)
	}
}

func TestMintWidthTwo(t *testing.T) {
	got, err := mint(2, map[string]string{}, 0, false)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if len(got) != 11 {
		t.Fatalf("len(%q) = %d, want 11", got, len(got))
	}
	if strings.Count(got, "-") != 1 {
		t.Fatalf("hyphen count in %q = %d, want 1", got, strings.Count(got, "-"))
	}
}

func TestMintDryRunCollidesErrors(t *testing.T) {
	first, err := mint(1, map[string]string{}, 42, true)
	if err != nil {
		t.Fatalf("seed dry-run: %v", err)
	}
	_, err = mint(1, map[string]string{first: "fake/path.md"}, 42, true)
	if err == nil {
		t.Errorf("dry-run with seeded collision: want error, got nil")
	}
}

func TestScanCorpusFindsCoordinationHandles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/thought-experiments/TE-vapoj-substrate.md", "# placeholder\n")
	writeFile(t, root, "TODO/TODO-bahor-something.md", "# placeholder\n\nID: DI-nisam\n")

	corpus, err := scanCorpus(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, handle := range []string{"vapoj", "bahor", "nisam"} {
		if _, ok := corpus[handle]; !ok {
			t.Errorf("missing handle %q in %v", handle, corpus)
		}
	}
}

func TestScanCorpusIgnoresLegacyFilenames(t *testing.T) {
	root := t.TempDir()
	legacy := []string{
		"docs/thought-experiments/TE-20260427-180000-promise-stack-ordering.md",
		"TODO/005-grid-workspace-tool-proposal.md",
		"TODO/J025-self-host-ntfy.md",
	}
	for _, path := range legacy {
		writeFile(t, root, path, "# placeholder\n")
	}
	corpus, err := scanCorpus(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(corpus) != 0 {
		t.Fatalf("legacy corpus size = %d, want 0 (%v)", len(corpus), corpus)
	}
}

func TestScanCorpusDetectsDuplicateHandles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/thought-experiments/TE-vapoj-something.md", "# placeholder\n")
	writeFile(t, root, "TODO/TODO-vapoj-something-else.md", "# placeholder\n")
	if _, err := scanCorpus(root); err == nil {
		t.Errorf("duplicate filename handle: want error, got nil")
	}
}

func writeFile(t *testing.T, root, path, contents string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
