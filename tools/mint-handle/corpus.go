package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// scanLiteralDirs are the decomk directories that own proquint handles.
// Missing directories are skipped so the tool still works in a partially
// migrated checkout or in focused tests.
//
// Intent: Treat the decomk coordination corpus itself as the registry for TODO,
// TE, and DI handles, avoiding a central sequence allocator. Source: DI-puhon
var scanLiteralDirs = []string{
	"TODO",
	"docs/thought-experiments",
}

// handleFileRE matches handle-bearing filenames and captures the handle.
// Filename grammar is KIND-HANDLE-SLUG.md where KIND is TODO or TE. DI handles
// are owned by inline Decision Intent Log entries in TODO files.
var handleFileRE = regexp.MustCompile(
	`^(?:TODO|TE)-(` +
		`[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]` +
		`-` +
		`[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]` +
		`|` +
		`[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]` +
		`)-`)

// diOwnerRE matches DI owner lines, not ordinary prose references. This keeps
// the DI handle namespace global without treating every citation as an owner.
var diOwnerRE = regexp.MustCompile(`(?m)^(?:ID|DI-ID):\s*DI-(` +
	`[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz]` +
	`(?:-[bdfghjklmnprstvz][aiou][bdfghjklmnprstvz][aiou][bdfghjklmnprstvz])?` +
	`)\s*$`)

// scanCorpus returns the set of handles currently owned by decomk
// records. Legacy numeric and timestamp filenames are intentionally ignored;
// only migrated records and proquint DI owner lines participate.
func scanCorpus(repoRoot string) (map[string]string, error) {
	handles := make(map[string]string)
	for _, dir := range scanLiteralDirs {
		full := filepath.Join(repoRoot, dir)
		info, err := os.Stat(full)
		switch {
		case err == nil && info.IsDir():
			if err := scanDir(repoRoot, dir, handles); err != nil {
				return nil, err
			}
		case err == nil:
			return nil, fmt.Errorf("scan path %s is not a directory", dir)
		case os.IsNotExist(err):
			continue
		default:
			return nil, fmt.Errorf("stat %s: %w", dir, err)
		}
	}
	return handles, nil
}

// scanDir records filename-owned handles and DI owner handles from one
// configured directory. Subdirectories are ignored because this repo keeps the
// migrated record layout flat.
func scanDir(repoRoot, dir string, handles map[string]string) error {
	full := filepath.Join(repoRoot, dir)
	entries, err := os.ReadDir(full)
	if err != nil {
		return fmt.Errorf("scan %s: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(dir, entry.Name()))
		if match := handleFileRE.FindStringSubmatch(entry.Name()); match != nil {
			if err := rememberHandle(handles, match[1], relPath); err != nil {
				return err
			}
		}
		if dir == "TODO" {
			if err := scanDIHandles(filepath.Join(full, entry.Name()), relPath, handles); err != nil {
				return err
			}
		}
	}
	return nil
}

// scanDIHandles records handles owned by inline Decision Intent Log entries in
// TODO files.
func scanDIHandles(path, relPath string, handles map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", relPath, err)
	}
	for _, match := range diOwnerRE.FindAllStringSubmatch(string(data), -1) {
		if err := rememberHandle(handles, match[1], relPath+"#DI-"+match[1]); err != nil {
			return err
		}
	}
	return nil
}

// rememberHandle inserts one owned handle into the corpus and reports duplicate
// owners clearly enough for a human to resolve a merge-time collision.
func rememberHandle(handles map[string]string, handle, owner string) error {
	if previous, duplicate := handles[handle]; duplicate {
		return fmt.Errorf("corpus already contains duplicate handle %q in %s and %s", handle, previous, owner)
	}
	handles[handle] = owner
	return nil
}
