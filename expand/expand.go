// Package expand implements isconf-style macro expansion for decomk.
//
// Expansion is purely textual:
//   - If a token exactly matches a key in Defs, it is replaced by that key's
//     token list, recursively.
//   - Unknown tokens are left as-is (isconf behavior).
//
// The implementation adds guardrails that are easy to unit test:
//   - cycle detection
//   - maximum expansion depth
package expand

import (
	"fmt"
	"strings"
)

// Defs maps a macro name to a list of tokens.
// It is compatible with contexts.Defs but duplicated here to avoid import
// cycles; callers can use a plain map literal or type conversion.
type Defs map[string][]string

// Options controls macro expansion guardrails.
type Options struct {
	// MaxDepth limits recursive expansion depth. If zero, a default is used.
	MaxDepth int
}

// ExpandTokens expands any macro tokens found in tokens.
func ExpandTokens(defs Defs, tokens []string, opts Options) ([]string, error) {
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 64
	}

	visiting := make(map[string]bool, len(defs))
	var stack []string

	var expandKey func(key string, depth int) ([]string, error)
	expandKey = func(key string, depth int) ([]string, error) {
		if depth > maxDepth {
			return nil, fmt.Errorf("max expansion depth exceeded (%d) while expanding %q", maxDepth, key)
		}
		if visiting[key] {
			chain := append(append([]string(nil), stack...), key)
			return nil, fmt.Errorf("macro cycle detected: %s", strings.Join(chain, " -> "))
		}

		body, ok := defs[key]
		if !ok {
			// Unknown macros are treated as literals by design.
			return []string{key}, nil
		}

		visiting[key] = true
		stack = append(stack, key)

		var out []string
		for _, tok := range body {
			if _, isMacro := defs[tok]; isMacro {
				expanded, err := expandKey(tok, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, expanded...)
				continue
			}
			out = append(out, tok)
		}

		stack = stack[:len(stack)-1]
		visiting[key] = false
		return out, nil
	}

	var out []string
	for _, tok := range tokens {
		if _, isMacro := defs[tok]; isMacro {
			expanded, err := expandKey(tok, 1)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded...)
			continue
		}
		out = append(out, tok)
	}
	return out, nil
}
