// Package contexts loads and parses decomk.conf-style context definitions.
//
// A decomk config file is a map from a context key to a list of tokens. Tokens
// are later expanded as macros (by name) and then partitioned into make targets
// and VAR=value tuples.
//
// The grammar is intentionally small and deterministic so it can be parsed
// safely without eval-like behavior:
//   - Whole-line comments start with '#'.
//   - Key lines are of the form:   key: token token token
//   - Continuation lines append more tokens to the most recent key.
//   - Tokens are whitespace-separated shell-words; single quotes may be used
//     to include spaces inside a token (quotes are removed while parsing).
package contexts

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Defs maps a context/macro name to its token list.
type Defs map[string][]string

// LoadTree loads a base config file and any sibling *.conf files in a matching
// "<basename>.d" directory (e.g., decomk.conf + decomk.d/*.conf).
//
// Later files override earlier ones by key (last definition wins).
func LoadTree(path string) (Defs, error) {
	base, err := LoadFile(path)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dDir := filepath.Join(dir, baseName+".d")

	info, err := os.Stat(dDir)
	if err != nil {
		// If the directory doesn't exist, that's fine; return just the base file.
		if os.IsNotExist(err) {
			return base, nil
		}
		return nil, fmt.Errorf("stat %q: %w", dDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q exists but is not a directory", dDir)
	}

	entries, err := os.ReadDir(dDir)
	if err != nil {
		return nil, fmt.Errorf("readdir %q: %w", dDir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".conf" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	defs := base
	for _, name := range names {
		p := filepath.Join(dDir, name)
		part, err := LoadFile(p)
		if err != nil {
			return nil, err
		}
		defs = Merge(defs, part)
	}
	return defs, nil
}

// LoadFile loads and parses a single config file.
func LoadFile(path string) (Defs, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	defs, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return defs, nil
}

// Parse parses decomk.conf content from r.
func Parse(r io.Reader) (Defs, error) {
	defs := make(Defs)

	scanner := bufio.NewScanner(r)
	// Allow moderately long lines for large token lists.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentKey string
	for lineNum := 1; scanner.Scan(); lineNum++ {
		raw := strings.TrimRight(scanner.Text(), "\r")

		trimLeft := strings.TrimLeftFunc(raw, unicode.IsSpace)
		if trimLeft == "" {
			continue
		}
		if strings.HasPrefix(trimLeft, "#") {
			continue
		}

		if key, rest, ok := splitKeyLine(trimLeft); ok {
			currentKey = key
			toks, err := splitTokens(rest)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			// Within a single file, the last definition of a key wins.
			defs[currentKey] = toks
			continue
		}

		// Continuation line.
		if currentKey == "" {
			return nil, fmt.Errorf("line %d: continuation line without a preceding key", lineNum)
		}
		toks, err := splitTokens(trimLeft)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		defs[currentKey] = append(defs[currentKey], toks...)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return defs, nil
}

// Merge returns a new Defs where overlay keys replace base keys.
func Merge(base, overlay Defs) Defs {
	out := make(Defs, len(base)+len(overlay))
	for k, v := range base {
		out[k] = append([]string(nil), v...)
	}
	for k, v := range overlay {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func splitKeyLine(line string) (key, rest string, ok bool) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", "", false
	}
	// Avoid mis-parsing tokens like "http://..." or "URL=https://...".
	if colon+1 < len(line) && !isSpace(rune(line[colon+1])) {
		return "", "", false
	}
	key = strings.TrimSpace(line[:colon])
	if key == "" {
		return "", "", false
	}
	// Keys are macro names; forbid '=' so VAR=value doesn't look like a key.
	if strings.ContainsRune(key, '=') {
		return "", "", false
	}
	rest = strings.TrimSpace(line[colon+1:])
	return key, rest, true
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// splitTokens splits a line into tokens using a minimal, explicit quoting rule:
// single quotes keep everything literal (including spaces), and are removed.
//
// Backslash escapes the next rune when not in single quotes.
func splitTokens(s string) ([]string, error) {
	var tokens []string
	var b strings.Builder

	inSingle := false
	escape := false

	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, b.String())
		b.Reset()
	}

	for _, r := range s {
		if escape {
			b.WriteRune(r)
			escape = false
			continue
		}

		if inSingle {
			if r == '\'' {
				inSingle = false
				continue
			}
			b.WriteRune(r)
			continue
		}

		switch {
		case r == '\\':
			escape = true
		case r == '\'':
			inSingle = true
		case isSpace(r):
			flush()
		default:
			b.WriteRune(r)
		}
	}

	if escape {
		return nil, fmt.Errorf("dangling backslash escape")
	}
	if inSingle {
		return nil, fmt.Errorf("unterminated single-quoted string")
	}
	flush()
	return tokens, nil
}
