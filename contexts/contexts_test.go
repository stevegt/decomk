package contexts

import (
	"strings"
	"testing"
)

func TestParse_BasicAndContinuation(t *testing.T) {
	t.Parallel()

	in := `
# comment
DEFAULT: Block00_base
  Block10_common
  FOO='bar baz'

grokker: DEFAULT Block20_go
`

	defs, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if got, want := strings.Join(defs["DEFAULT"], "|"), "Block00_base|Block10_common|FOO=bar baz"; got != want {
		t.Fatalf("DEFAULT tokens: got %q want %q", got, want)
	}
	if got, want := strings.Join(defs["grokker"], "|"), "DEFAULT|Block20_go"; got != want {
		t.Fatalf("grokker tokens: got %q want %q", got, want)
	}
}

func TestParse_ContinuationWithoutKeyIsError(t *testing.T) {
	t.Parallel()

	_, err := Parse(strings.NewReader("  Block00_base\n"))
	if err == nil {
		t.Fatalf("Parse() expected error, got nil")
	}
}

func TestParse_UnterminatedQuoteIsError(t *testing.T) {
	t.Parallel()

	_, err := Parse(strings.NewReader("DEFAULT: FOO='bar\n"))
	if err == nil {
		t.Fatalf("Parse() expected error, got nil")
	}
}

func TestParse_DoesNotMisparseURLLikeTokensAsKeys(t *testing.T) {
	t.Parallel()

	in := `
DEFAULT:
  URL=https://example.com/path
  http://example.com/also-ok
`

	defs, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if got, want := strings.Join(defs["DEFAULT"], "|"), "URL=https://example.com/path|http://example.com/also-ok"; got != want {
		t.Fatalf("DEFAULT tokens: got %q want %q", got, want)
	}
}
