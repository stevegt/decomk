// Command sweep-citations rewrites current decomk legacy ID references to
// proquint IDs using tools/migrate-handles/mapping.tsv.
//
// It updates text-like repo files because decomk references appear in
// documentation, behavior comments, scripts, JSON, Makefiles, and configs. The
// mapping TSV and root xref are skipped so old IDs remain available for lookup.
//
// Intent: Make the proquint migration repo-wide while preserving the explicit
// legacy lookup artifacts. Source: DI-puhon
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const mappingPath = "tools/migrate-handles/mapping.tsv"

type row struct {
	Handle  string
	Kind    string
	OldID   string
	NewID   string
	OldPath string
	NewPath string
	Title   string
}

type intRep struct {
	re *regexp.Regexp
	to string
}

type literalRep struct {
	from string
	to   string
}

func main() {
	repoRoot := flag.String("r", ".", "decomk repo root")
	dryRun := flag.Bool("n", false, "dry-run")
	quiet := flag.Bool("q", false, "suppress per-file progress")
	flag.Parse()

	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		die("repo root: %v", err)
	}
	rows, err := loadMapping(filepath.Join(root, mappingPath))
	if err != nil {
		die("load mapping: %v", err)
	}
	intReps, literalReps := buildReplacements(rows)
	var scanned, changed int
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if shouldSkipDir(root, path) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if shouldSkipFile(rel) || !isSweepable(rel) {
			return nil
		}
		scanned++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		newBody, edits := sweep(string(body), intReps, literalReps)
		if edits == 0 {
			return nil
		}
		changed++
		if !*quiet {
			fmt.Fprintf(os.Stderr, "  %s  %d edits\n", rel, edits)
		}
		if !*dryRun {
			if err := os.WriteFile(path, []byte(newBody), info.Mode()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		die("walk: %v", err)
	}
	fmt.Fprintf(os.Stderr, "sweep-citations: scanned=%d, changed=%d, dry-run=%v\n", scanned, changed, *dryRun)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sweep-citations: "+format+"\n", args...)
	os.Exit(1)
}

func loadMapping(path string) ([]row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(f)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return nil, fmt.Errorf("read mapping: %w; close mapping: %v", err, closeErr)
		}
		return nil, fmt.Errorf("read mapping: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close mapping: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("empty mapping")
	}
	var rows []row
	for i, rec := range records[1:] {
		if len(rec) < 7 {
			return nil, fmt.Errorf("mapping row %d has %d fields, want 7", i+2, len(rec))
		}
		rows = append(rows, row{Handle: rec[0], Kind: rec[1], OldID: rec[2], NewID: rec[3], OldPath: rec[4], NewPath: rec[5], Title: rec[6]})
	}
	return rows, nil
}

func buildReplacements(rows []row) ([]intRep, []literalRep) {
	var intReps []intRep
	var literalReps []literalRep
	// Literal replacements handle exact IDs and paths first; regex replacements
	// then handle human prose forms like "TODO-mirut.2" without touching ordinary
	// operational numbers. Source: DI-puhon
	for _, r := range rows {
		literalReps = append(literalReps, literalRep{from: r.OldID, to: r.NewID})
		if !strings.Contains(r.OldPath, "#") {
			literalReps = append(literalReps, literalRep{from: r.OldPath, to: r.NewPath})
			literalReps = append(literalReps, literalRep{from: filepath.Base(r.OldPath), to: filepath.Base(r.NewPath)})
		}
		if r.Kind == "TODO" {
			old := strings.TrimPrefix(r.OldID, "TODO-")
			if len(old) == 3 && isDigits(old) {
				bare := strings.TrimLeft(old, "0")
				if bare == "" {
					bare = "0"
				}
				pattern := `(^|[^\w-])TODO[\s/-]+0*` + regexp.QuoteMeta(bare) + `(\.[0-9]+)?($|[^\w-])`
				intReps = append(intReps, intRep{re: regexp.MustCompile(pattern), to: r.NewID})
			} else {
				pattern := `(^|[^\w-])TODO[\s/-]+` + regexp.QuoteMeta(old) + `(\.[0-9]+)?($|[^\w-])`
				intReps = append(intReps, intRep{re: regexp.MustCompile(pattern), to: r.NewID})
			}
		}
	}
	sort.Slice(literalReps, func(i, j int) bool { return len(literalReps[i].from) > len(literalReps[j].from) })
	sort.Slice(intReps, func(i, j int) bool { return len(intReps[i].re.String()) > len(intReps[j].re.String()) })
	return intReps, literalReps
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func sweep(body string, intReps []intRep, literalReps []literalRep) (string, int) {
	edits := 0
	// Longest literal replacements run first so filenames are rewritten as a
	// whole before shorter ID fragments can match inside those paths. Source:
	// DI-puhon
	for _, rep := range literalReps {
		count := strings.Count(body, rep.from)
		if count == 0 {
			continue
		}
		body = strings.ReplaceAll(body, rep.from, rep.to)
		edits += count
	}
	for _, rep := range intReps {
		body = rep.re.ReplaceAllStringFunc(body, func(match string) string {
			parts := rep.re.FindStringSubmatch(match)
			if len(parts) != 4 {
				return match
			}
			edits++
			return parts[1] + rep.to + parts[2] + parts[3]
		})
	}
	return body, edits
}

func shouldSkipDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel == ".git" || strings.HasPrefix(rel, ".git/")
}

func shouldSkipFile(rel string) bool {
	return rel == mappingPath || rel == "numeric-proquint-xref.md"
}

func isSweepable(rel string) bool {
	base := filepath.Base(rel)
	if base == "Makefile" || base == "Dockerfile" || base == "AGENTS.md" {
		return true
	}
	return strings.HasSuffix(rel, ".md") ||
		strings.HasSuffix(rel, ".go") ||
		strings.HasSuffix(rel, ".csv") ||
		strings.HasSuffix(rel, ".sh") ||
		strings.HasSuffix(rel, ".json") ||
		strings.HasSuffix(rel, ".conf") ||
		strings.HasSuffix(rel, ".tmpl")
}
