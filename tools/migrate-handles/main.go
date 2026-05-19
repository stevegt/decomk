// Command migrate-handles performs the one-time decomk proquint migration.
//
// It discovers legacy TODO files, TE files, and inline DI records, assigns each a proquint
// handle, writes tools/migrate-handles/mapping.tsv, renames filename-owned
// records with git mv or an approved plain filesystem rename fallback, and
// updates record-local titles/ID fields plus the root numeric-proquint xref.
//
// Intent: Make the ID migration repeatable and auditable instead of relying on
// hand renames. Source: DI-puhon
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	mappingPath = "tools/migrate-handles/mapping.tsv"
	xrefPath    = "numeric-proquint-xref.md"
)

var (
	legacyTODOFileRE     = regexp.MustCompile(`^([0-9]{3}|[A-Z][0-9]{3})-(.+)\.md$`)
	legacyTEFileRE       = regexp.MustCompile(`^TE-([0-9]{8}-[0-9]{6})-(.+)\.md$`)
	inlineDIIDRE         = regexp.MustCompile(`(?m)^ID:\s*(DI-[0-9]{3}-[0-9]{8}-[0-9]{6})\s*$`)
	proquintHandleRE     = regexp.MustCompile(`^[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz](?:-[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz])?$`)
	existingHandleFileRE = regexp.MustCompile(`^(?:TODO|TE)-(` +
		`[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]` +
		`(?:-[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz])?` +
		`)-`)
	existingDIOwnerRE = regexp.MustCompile(`(?m)^(?:ID|DI-ID):\s*DI-(` +
		`[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]` +
		`(?:-[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz])?` +
		`)\s*$`)
)

// MappingRow is one old-to-new record identity mapping. The TSV form is the
// authority for later citation sweeps and human xref generation.
type MappingRow struct {
	Handle  string
	Kind    string
	OldID   string
	NewID   string
	OldPath string
	NewPath string
	Title   string
}

func main() {
	repoRoot := flag.String("r", ".", "decomk repo root")
	dryRun := flag.Bool("n", false, "dry-run; print actions without changing files")
	quiet := flag.Bool("q", false, "suppress per-record progress")
	plainRename := flag.Bool("plain-rename", false, "use os.Rename instead of git mv when .git/index is unavailable")
	flag.Parse()

	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		die("repo root: %v", err)
	}
	rows, err := discoverRows(root)
	if err != nil {
		die("discover: %v", err)
	}
	if len(rows) == 0 {
		die("no legacy records found")
	}
	if err := assignHandles(root, rows); err != nil {
		die("assign handles: %v", err)
	}
	if err := fillNewPaths(rows); err != nil {
		die("new paths: %v", err)
	}
	if !*quiet {
		for _, row := range rows {
			fmt.Fprintf(os.Stderr, "%s -> %s  %s -> %s\n", row.OldID, row.NewID, row.OldPath, row.NewPath)
		}
	}
	if *dryRun {
		fmt.Fprintf(os.Stderr, "migrate-handles: dry-run rows=%d\n", len(rows))
		return
	}
	if err := writeMapping(root, rows); err != nil {
		die("write mapping: %v", err)
	}
	if err := writeXref(root, rows); err != nil {
		die("write xref: %v", err)
	}
	if err := renameFiles(root, rows, *plainRename); err != nil {
		die("rename: %v", err)
	}
	if err := updateMigratedRecords(root, rows); err != nil {
		die("update records: %v", err)
	}
	fmt.Fprintf(os.Stderr, "migrate-handles: migrated %d records\n", len(rows))
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "migrate-handles: "+format+"\n", args...)
	os.Exit(1)
}

func discoverRows(root string) ([]*MappingRow, error) {
	var rows []*MappingRow
	// addFileRows captures filename-owned legacy records before inline DIs so
	// inline DI rows can later point at the migrated TODO file path. Source: DI-puhon
	addFileRows := func(dir string, re *regexp.Regexp, kind string, oldID func(string) string) error {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("read %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			m := re.FindStringSubmatch(entry.Name())
			if m == nil {
				continue
			}
			oldPath := filepath.ToSlash(filepath.Join(dir, entry.Name()))
			title, err := titleFromFile(filepath.Join(root, oldPath), kind, m[2])
			if err != nil {
				return err
			}
			rows = append(rows, &MappingRow{Kind: kind, OldID: oldID(m[1]), OldPath: oldPath, Title: title})
		}
		return nil
	}
	if err := addFileRows("TODO", legacyTODOFileRE, "TODO", func(s string) string { return "TODO-" + s }); err != nil {
		return nil, err
	}
	if err := addFileRows("docs/thought-experiments", legacyTEFileRE, "TE", func(s string) string { return "TE-" + s }); err != nil {
		return nil, err
	}
	rows, err := discoverInlineDIs(root, rows)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Kind != rows[j].Kind {
			return kindRank(rows[i].Kind) < kindRank(rows[j].Kind)
		}
		return rows[i].OldID < rows[j].OldID
	})
	return rows, nil
}

func discoverInlineDIs(root string, rows []*MappingRow) ([]*MappingRow, error) {
	entries, err := os.ReadDir(filepath.Join(root, "TODO"))
	if err != nil {
		return nil, fmt.Errorf("read TODO: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "TODO.md" {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("TODO", entry.Name()))
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		for _, match := range inlineDIIDRE.FindAllStringSubmatch(string(body), -1) {
			rows = append(rows, &MappingRow{
				Kind:    "DI",
				OldID:   match[1],
				OldPath: rel + "#" + match[1],
				Title:   titleForInlineDI(string(body), match[1]),
			})
		}
	}
	return rows, nil
}

func kindRank(kind string) int {
	switch kind {
	case "TODO":
		return 0
	case "TE":
		return 1
	case "DI":
		return 2
	default:
		return 9
	}
}

func titleFromFile(path, kind, slug string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			title = legacyTitleSuffix(title, kind)
			if title != "" {
				return title, nil
			}
			break
		}
	}
	return titleFromSlug(slug), nil
}

func legacyTitleSuffix(title, kind string) string {
	if strings.HasPrefix(title, kind+" ") {
		parts := strings.SplitN(title, " - ", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}
	if strings.HasPrefix(title, kind+"-") {
		parts := strings.SplitN(title, " - ", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}
	if kind == "TODO" {
		numeric := regexp.MustCompile(`^[0-9]{3}\s+-\s+(.+)$`)
		if match := numeric.FindStringSubmatch(title); match != nil {
			return strings.TrimSpace(match[1])
		}
	}
	return title
}

func titleFromSlug(slug string) string {
	parts := strings.Split(strings.TrimSuffix(slug, ".md"), "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func titleForInlineDI(body, oldID string) string {
	idx := strings.Index(body, "ID: "+oldID)
	if idx < 0 {
		return "Inline decision intent"
	}
	section := body[idx:]
	if next := strings.Index(section, "\nID: DI-"); next > 0 {
		section = section[:next]
	}
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "Decision:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Decision:"))
		}
	}
	return "Inline decision intent"
}

func assignHandles(root string, rows []*MappingRow) error {
	claimed, err := scanExistingHandles(root)
	if err != nil {
		return err
	}
	repoSeed := filepath.Base(root)
	for _, row := range rows {
		handle, err := mintDeterministic(repoSeed+":"+row.OldID, claimed)
		if err != nil {
			return err
		}
		row.Handle = handle
		row.NewID = row.Kind + "-" + handle
		claimed[handle] = row.OldID
	}
	return nil
}

func scanExistingHandles(root string) (map[string]string, error) {
	claimed := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".git/") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".md") {
			return nil
		}
		base := filepath.Base(rel)
		if match := existingHandleFileRE.FindStringSubmatch(base); match != nil {
			claimed[match[1]] = rel
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range existingDIOwnerRE.FindAllStringSubmatch(string(body), -1) {
			claimed[match[1]] = rel + "#DI-" + match[1]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func mintDeterministic(seed string, claimed map[string]string) (string, error) {
	for attempt := 0; attempt < 1_000_000; attempt++ {
		payload := fmt.Sprintf("%s#%d", seed, attempt)
		sum := sha256.Sum256([]byte(payload))
		candidate := proquint1FromBytes(sum[:2])
		if !proquintHandleRE.MatchString(candidate) {
			return "", fmt.Errorf("candidate %q is not a proquint", candidate)
		}
		if _, ok := claimed[candidate]; !ok {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("exhausted deterministic handle attempts for %s", seed)
}

func fillNewPaths(rows []*MappingRow) error {
	fileHandleByOldPath := map[string]string{}
	// Filename-owned records get their destination first. Inline DIs then reuse
	// their parent TODO destination and replace only the fragment identifier.
	// Source: DI-puhon
	for _, row := range rows {
		if strings.Contains(row.OldPath, "#") {
			continue
		}
		fileHandleByOldPath[row.OldPath] = row.Handle
		slug := slugFromLegacyPath(row.OldPath, row.Kind)
		switch row.Kind {
		case "TODO":
			row.NewPath = "TODO/" + row.NewID + "-" + slug + ".md"
		case "TE":
			row.NewPath = "docs/thought-experiments/" + row.NewID + "-" + slug + ".md"
		default:
			return fmt.Errorf("unknown kind %s", row.Kind)
		}
	}
	for _, row := range rows {
		if !strings.Contains(row.OldPath, "#") {
			continue
		}
		parts := strings.SplitN(row.OldPath, "#", 2)
		baseNew, ok := fileNewPath(rows, parts[0])
		if !ok {
			return fmt.Errorf("inline DI %s points at unmapped file %s", row.OldID, parts[0])
		}
		row.NewPath = baseNew + "#" + row.NewID
	}
	return nil
}

func fileNewPath(rows []*MappingRow, oldPath string) (string, bool) {
	for _, row := range rows {
		if row.OldPath == oldPath {
			return row.NewPath, true
		}
	}
	return "", false
}

func slugFromLegacyPath(oldPath, kind string) string {
	base := strings.TrimSuffix(filepath.Base(oldPath), ".md")
	switch kind {
	case "TODO":
		if m := legacyTODOFileRE.FindStringSubmatch(filepath.Base(oldPath)); m != nil {
			return m[2]
		}
	case "TE":
		if m := legacyTEFileRE.FindStringSubmatch(filepath.Base(oldPath)); m != nil {
			return m[2]
		}
	}
	return base
}

func writeMapping(root string, rows []*MappingRow) error {
	full := filepath.Join(root, mappingPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(f)
	writer.Comma = '\t'
	if err := writer.Write([]string{"handle", "kind", "old_id", "new_id", "old_path", "new_path", "title"}); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return fmt.Errorf("write mapping header: %w; close mapping: %v", err, closeErr)
		}
		return fmt.Errorf("write mapping header: %w", err)
	}
	for _, row := range rows {
		if err := writer.Write([]string{row.Handle, row.Kind, row.OldID, row.NewID, row.OldPath, row.NewPath, row.Title}); err != nil {
			if closeErr := f.Close(); closeErr != nil {
				return fmt.Errorf("write mapping row %s: %w; close mapping: %v", row.OldID, err, closeErr)
			}
			return fmt.Errorf("write mapping row %s: %w", row.OldID, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return fmt.Errorf("flush mapping: %w; close mapping: %v", err, closeErr)
		}
		return fmt.Errorf("flush mapping: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close mapping: %w", err)
	}
	return nil
}

func writeXref(root string, rows []*MappingRow) error {
	// The xref deliberately duplicates the TSV in a human-friendly Markdown
	// table so old chat references and editor searches can be resolved without
	// remembering the TSV column order. Source: DI-puhon
	var b strings.Builder
	b.WriteString("# Numeric to proquint ID cross-reference\n\n")
	b.WriteString("This file records the one-time decomk migration from numeric/timestamp coordination IDs to proquint IDs.\n")
	b.WriteString("Use this file only for historical lookup; new records should use `tools/mint-handle` and proquint IDs directly.\n\n")
	b.WriteString("Source of truth: `tools/migrate-handles/mapping.tsv`.\n\n")
	b.WriteString("| old ID | new ID | old path | new path | title |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | %s |\n",
			row.OldID, row.NewID, row.OldPath, row.NewPath, escapeMarkdownTable(row.Title))
	}
	return os.WriteFile(filepath.Join(root, xrefPath), []byte(b.String()), 0o644)
}

func escapeMarkdownTable(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func renameFiles(root string, rows []*MappingRow, plainRename bool) error {
	for _, row := range rows {
		if strings.Contains(row.OldPath, "#") || row.OldPath == row.NewPath {
			continue
		}
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(row.NewPath)), 0o755); err != nil {
			return err
		}
		if plainRename {
			if err := os.Rename(filepath.Join(root, row.OldPath), filepath.Join(root, row.NewPath)); err != nil {
				return fmt.Errorf("rename %s %s: %w", row.OldPath, row.NewPath, err)
			}
		} else {
			cmd := exec.Command("git", "-C", root, "mv", row.OldPath, row.NewPath)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("git mv %s %s: %w\n%s", row.OldPath, row.NewPath, err, out)
			}
		}
	}
	return nil
}

func updateMigratedRecords(root string, rows []*MappingRow) error {
	idReps := map[string]string{}
	rowsByNewPath := map[string][]*MappingRow{}
	for _, row := range rows {
		idReps[row.OldID] = row.NewID
		filePath := strings.SplitN(row.NewPath, "#", 2)[0]
		rowsByNewPath[filePath] = append(rowsByNewPath[filePath], row)
	}
	for newPath, fileRows := range rowsByNewPath {
		full := filepath.Join(root, newPath)
		bodyBytes, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("read migrated %s: %w", newPath, err)
		}
		body := string(bodyBytes)
		for _, row := range fileRows {
			body = strings.ReplaceAll(body, row.OldID, row.NewID)
		}
		owner := fileRows[0]
		if !strings.Contains(owner.OldPath, "#") {
			body = replaceH1(body, owner.NewID, owner.Title)
			if owner.Kind == "TODO" {
				body = rewriteOwnSubtasks(body, owner.OldID, owner.Handle)
			}
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write migrated %s: %w", newPath, err)
		}
	}
	return nil
}

func replaceH1(body, newID, title string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			lines[i] = "# " + newID + ": " + title
			return strings.Join(lines, "\n")
		}
	}
	return "# " + newID + ": " + title + "\n\n" + body
}

func rewriteOwnSubtasks(body, oldID, handle string) string {
	old := strings.TrimPrefix(oldID, "TODO-")
	candidates := []string{old}
	if len(old) == 4 && old[0] >= 'A' && old[0] <= 'Z' {
		candidates = append(candidates, old[1:])
	}
	for _, candidate := range candidates {
		bare := strings.TrimLeft(candidate, "0")
		if bare == "" {
			bare = "0"
		}
		re := regexp.MustCompile(`\b0*` + regexp.QuoteMeta(bare) + `\.(\d+)`)
		body = re.ReplaceAllString(body, handle+".$1")
	}
	return body
}

// Proquint encoding is duplicated here to keep this one-shot tool independent
// from tools/mint-handle as a separate Go module.
const (
	proquintCons = "bdfghjklmnprstvz"
	proquintVows = "aiou"
)

func uint16ToProquint(n uint16) string {
	buf := []byte{
		proquintCons[(n>>12)&0x0f],
		proquintVows[(n>>10)&0x03],
		proquintCons[(n>>6)&0x0f],
		proquintVows[(n>>4)&0x03],
		proquintCons[n&0x0f],
	}
	return string(buf)
}

func proquint1FromBytes(b []byte) string {
	n := binary.BigEndian.Uint16(b[:2])
	return uint16ToProquint(n)
}
