package expand

import (
	"strings"
	"testing"
)

func TestExpandTokens_Recursive(t *testing.T) {
	t.Parallel()

	defs := Defs{
		"DEFAULT": {"A", "B"},
		"grokker": {"DEFAULT", "C"},
	}

	out, err := ExpandTokens(defs, []string{"DEFAULT", "grokker"}, Options{})
	if err != nil {
		t.Fatalf("ExpandTokens() error: %v", err)
	}
	if got, want := strings.Join(out, "|"), "A|B|A|B|C"; got != want {
		t.Fatalf("out: got %q want %q", got, want)
	}
}

func TestExpandTokens_UnknownTokensRemainLiteral(t *testing.T) {
	t.Parallel()

	defs := Defs{
		"DEFAULT": {"A"},
	}

	out, err := ExpandTokens(defs, []string{"DEFAULT", "UNKNOWN"}, Options{})
	if err != nil {
		t.Fatalf("ExpandTokens() error: %v", err)
	}
	if got, want := strings.Join(out, "|"), "A|UNKNOWN"; got != want {
		t.Fatalf("out: got %q want %q", got, want)
	}
}

func TestExpandTokens_Cycle(t *testing.T) {
	t.Parallel()

	defs := Defs{
		"A": {"B"},
		"B": {"A"},
	}

	_, err := ExpandTokens(defs, []string{"A"}, Options{})
	if err == nil {
		t.Fatalf("ExpandTokens() expected error, got nil")
	}
}

func TestExpandTokens_MaxDepth(t *testing.T) {
	t.Parallel()

	defs := Defs{
		"A": {"B"},
		"B": {"C"},
		"C": {"D"},
		"D": {"E"},
		"E": {"F"},
		"F": {"G"},
		"G": {"H"},
		"H": {"I"},
		"I": {"J"},
		"J": {"K"},
		"K": {"L"},
		"L": {"M"},
		"M": {"N"},
		"N": {"O"},
		"O": {"P"},
		"P": {"Q"},
		"Q": {"R"},
		"R": {"S"},
		"S": {"T"},
		"T": {"U"},
		"U": {"V"},
		"V": {"W"},
		"W": {"X"},
		"X": {"Y"},
		"Y": {"Z"},
		"Z": {"DONE"},
	}

	_, err := ExpandTokens(defs, []string{"A"}, Options{MaxDepth: 5})
	if err == nil {
		t.Fatalf("ExpandTokens() expected error, got nil")
	}
}
