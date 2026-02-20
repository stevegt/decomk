package resolve

import (
	"reflect"
	"testing"
)

func TestSplitTuple(t *testing.T) {
	t.Parallel()

	// SplitTuple only treats NAME=value as a tuple if NAME is identifier-like.
	// This prevents accidentally interpreting target names with punctuation.
	cases := []struct {
		in      string
		wantOK  bool
		wantKey string
		wantVal string
	}{
		{in: "FOO=bar", wantOK: true, wantKey: "FOO", wantVal: "bar"},
		{in: "x=1", wantOK: true, wantKey: "x", wantVal: "1"},
		{in: "_X=1", wantOK: true, wantKey: "_X", wantVal: "1"},
		{in: "FOO=bar baz", wantOK: true, wantKey: "FOO", wantVal: "bar baz"},
		{in: "1FOO=bar", wantOK: false},
		{in: "=bar", wantOK: false},
		{in: "FOO", wantOK: false},
		{in: "FOO+=", wantOK: false},
		{in: "FOO-BAR=baz", wantOK: false},
	}

	for _, tc := range cases {
		gotKey, gotVal, gotOK := SplitTuple(tc.in)
		if gotOK != tc.wantOK {
			t.Fatalf("SplitTuple(%q) ok: got %v want %v", tc.in, gotOK, tc.wantOK)
		}
		if !gotOK {
			continue
		}
		if gotKey != tc.wantKey || gotVal != tc.wantVal {
			t.Fatalf("SplitTuple(%q): got (%q,%q) want (%q,%q)", tc.in, gotKey, gotVal, tc.wantKey, tc.wantVal)
		}
	}
}

func TestPartition(t *testing.T) {
	t.Parallel()

	// Partition preserves relative order within each class (tuples/targets) and
	// is used to build make's argv.
	tuples, targets := Partition([]string{"Block00", "FOO=bar", "X=1", "Block10"})
	if want := []string{"FOO=bar", "X=1"}; !reflect.DeepEqual(tuples, want) {
		t.Fatalf("tuples: got %#v want %#v", tuples, want)
	}
	if want := []string{"Block00", "Block10"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets: got %#v want %#v", targets, want)
	}
}
