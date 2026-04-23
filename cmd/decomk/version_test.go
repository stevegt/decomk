package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdVersion_PrintsVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdVersion(nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdVersion() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdVersion() code: got %d want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr: got %q want empty", stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), decomkVersion; got != want {
		t.Fatalf("stdout version: got %q want %q", got, want)
	}
}

func TestCmdVersion_RejectsArgs(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdVersion([]string{"extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdVersion(extra) error: got nil want error")
	}
	if code != 2 {
		t.Fatalf("cmdVersion(extra) code: got %d want 2", code)
	}
	if !strings.Contains(err.Error(), "does not accept positional args") {
		t.Fatalf("cmdVersion(extra) error: got %q", err.Error())
	}
}

func TestRunVersionCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"decomk", "version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(version) code: got %d want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run(version) stderr: got %q want empty", stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), decomkVersion; got != want {
		t.Fatalf("run(version) output: got %q want %q", got, want)
	}
}
