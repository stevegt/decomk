package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stevegt/decomk/confrepo"
	"github.com/stevegt/decomk/stage0"
)

func TestImageOwnedGoToolchainPolicy(t *testing.T) {
	t.Parallel()

	// Intent: Lock the Go toolchain contract to Dockerfile/image configuration,
	// not stage0, so decomk bootstrap cannot silently download compilers during
	// container startup. Source: DI-fisof (TODO-vimoj)
	expectedSnippets := []string{
		"golang-1.23-go",
		"ENV PATH=/usr/lib/go-1.23/bin:$PATH",
		"ENV GOTOOLCHAIN=local",
		"ln -sf /usr/lib/go-1.23/bin/go /usr/local/bin/go",
		"ln -sf /usr/lib/go-1.23/bin/gofmt /usr/local/bin/gofmt",
	}
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "codespaces selftest Dockerfile",
			content: readTestFile(t, repoPathFromCmdDecomk(".devcontainer/codespaces-selftest/Dockerfile")),
		},
		{
			name: "rendered confrepo Dockerfile",
			content: renderTestTemplate(t, "confrepo.Dockerfile", initConfRepoDockerfileTemplate,
				confrepo.ProducerDevcontainerData("decomk conf producer example").EnsureDefaults()),
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, snippet := range expectedSnippets {
				if !strings.Contains(tc.content, snippet) {
					t.Fatalf("%s missing %q in:\n%s", tc.name, snippet, tc.content)
				}
			}
		})
	}
}

func TestStage0DoesNotOwnGOTOOLCHAINPolicy(t *testing.T) {
	t.Parallel()

	// Intent: Keep stage0 production-generic; images own compiler-download policy
	// via GOTOOLCHAIN=local so different image families can choose their own Go
	// package source without editing stage0. Source: DI-fisof (TODO-vimoj)
	if strings.Contains(initStage0ScriptTemplate, "GOTOOLCHAIN=local") {
		t.Fatalf("stage0 template must not set GOTOOLCHAIN=local")
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(content)
}

func renderTestTemplate(t *testing.T, name, templateText string, data any) string {
	t.Helper()
	content, err := stage0.RenderTemplate(name, templateText, data)
	if err != nil {
		t.Fatalf("RenderTemplate(%s): %v", name, err)
	}
	return string(content)
}
