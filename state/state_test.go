package state

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeComponent_IsSinglePathComponent(t *testing.T) {
	t.Parallel()

	// Context keys commonly look like "owner/repo". The derived component must not
	// contain a path separator, and must not be "." or "..".
	got := SafeComponent("stevegt/decomk")
	if got == "" {
		t.Fatalf("SafeComponent() returned empty string")
	}
	if strings.Contains(got, string(filepath.Separator)) {
		t.Fatalf("SafeComponent() contains path separator: %q", got)
	}
	if got == "." || got == ".." {
		t.Fatalf("SafeComponent() returned traversal component: %q", got)
	}
}

func TestSafeComponent_DoesNotReturnDotOrDotDot(t *testing.T) {
	t.Parallel()

	// "." and ".." are special components in path semantics. They must never be
	// returned, regardless of the input.
	if got := SafeComponent("."); got == "." || got == ".." {
		t.Fatalf("SafeComponent(%q) = %q; want not '.' or '..'", ".", got)
	}
	if got := SafeComponent(".."); got == "." || got == ".." {
		t.Fatalf("SafeComponent(%q) = %q; want not '.' or '..'", "..", got)
	}
}

func TestSafeComponent_ReducesAccidentalCollisions(t *testing.T) {
	t.Parallel()

	// These two strings sanitize to similar prefixes (depending on the rules),
	// but should still map to different components via the hash suffix.
	a := SafeComponent("a/b")
	b := SafeComponent("a_b")
	if a == b {
		t.Fatalf("SafeComponent produced a collision: %q == %q", a, b)
	}
}
