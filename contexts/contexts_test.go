package contexts

import (
	"strings"
	"testing"
)

func TestParse_BasicAndContinuation(t *testing.T) {
	t.Parallel()

	// Basic parse: key lines define tokens, and non-key lines append tokens to the
	// most recent key. Single quotes are removed while parsing, so the token value
	// can contain spaces without splitting.
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

	// A continuation line without any preceding key is ambiguous and should fail
	// fast with a line-numbered error.
	_, err := Parse(strings.NewReader("  Block00_base\n"))
	if err == nil {
		t.Fatalf("Parse() expected error, got nil")
	}
}

func TestParse_UnterminatedQuoteIsError(t *testing.T) {
	t.Parallel()

	// Single-quote strings must terminate on the same line.
	_, err := Parse(strings.NewReader("DEFAULT: FOO='bar\n"))
	if err == nil {
		t.Fatalf("Parse() expected error, got nil")
	}
}

func TestParse_DoesNotMisparseURLLikeTokensAsKeys(t *testing.T) {
	t.Parallel()

	// Lines/tokens that contain ':' (like http://...) must not be interpreted as
	// "key:" definitions. We require ':' to be followed by whitespace or EOL in
	// order to be considered a key line.
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

func TestValidateRefs(t *testing.T) {
	t.Parallel()

	t.Run("accepts tuple and known key refs", func(t *testing.T) {
		t.Parallel()
		defs := Defs{
			"DEFAULT": {"COMMON", "INSTALL=tool-a tool-b"},
			"COMMON":  {"FOO=bar"},
		}
		if err := ValidateRefs(defs); err != nil {
			t.Fatalf("ValidateRefs() error: %v", err)
		}
	})

	t.Run("rejects bare unknown token", func(t *testing.T) {
		t.Parallel()
		defs := Defs{
			"DEFAULT": {"UNKNOWN_TOKEN"},
		}
		err := ValidateRefs(defs)
		if err == nil {
			t.Fatalf("ValidateRefs() expected error, got nil")
		}
		if got, want := err.Error(), `invalid token "UNKNOWN_TOKEN" in key "DEFAULT"`; !strings.Contains(got, want) {
			t.Fatalf("ValidateRefs() error: got %q want substring %q", got, want)
		}
	})
}
