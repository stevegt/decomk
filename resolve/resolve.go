// Package resolve contains small, testable helpers for turning expanded tokens
// into the argv pieces that decomk passes to make.
package resolve

import "unicode"

// Partition splits tokens into make variable tuples (NAME=value) and make
// targets (everything else).
//
// Variable tuples must come before targets on make's argv.
func Partition(tokens []string) (tuples, targets []string) {
	for _, tok := range tokens {
		if _, _, ok := SplitTuple(tok); ok {
			tuples = append(tuples, tok)
			continue
		}
		targets = append(targets, tok)
	}
	return tuples, targets
}

// SplitTuple splits a token of the form NAME=value.
// It returns ok=false if the token is not a tuple.
func SplitTuple(token string) (name, value string, ok bool) {
	eq := -1
	for i := 0; i < len(token); i++ {
		if token[i] == '=' {
			eq = i
			break
		}
	}
	if eq <= 0 {
		return "", "", false
	}
	name = token[:eq]
	value = token[eq+1:]
	if !isIdent(name) {
		return "", "", false
	}
	return name, value, true
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
