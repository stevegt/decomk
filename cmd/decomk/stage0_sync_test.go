package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stevegt/decomk/stage0"
)

// repoPathFromCmdDecomk resolves a repo-relative path from cmd/decomk tests.
func repoPathFromCmdDecomk(rel string) string {
	return filepath.Clean(filepath.Join("..", "..", rel))
}

func TestGeneratedStage0FilesMatchTemplates(t *testing.T) {
	t.Parallel()

	// Intent: Enforce template/example parity in tests so production and selftest
	// stage-0 files cannot drift from the canonical init templates.
	// Source: DI-tikub (TODO-jirin)
	tests := []struct {
		name   string
		path   string
		mode   os.FileMode
		render func() ([]byte, error)
	}{
		{
			name: "example devcontainer json",
			path: "examples/devcontainer/devcontainer.json",
			mode: 0o644,
			render: func() ([]byte, error) {
				return stage0.RenderDevcontainerJSON(initDevcontainerJSONTemplate, stage0.ProductionExampleDevcontainerData())
			},
		},
		{
			name: "selftest devcontainer json",
			path: "examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json",
			mode: 0o644,
			render: func() ([]byte, error) {
				return stage0.RenderDevcontainerJSON(initDevcontainerJSONTemplate, stage0.SelftestDevcontainerData())
			},
		},
		{
			name: "example stage0 script",
			path: "examples/devcontainer/decomk-stage0.sh",
			mode: 0o755,
			render: func() ([]byte, error) {
				return stage0.RenderStage0Script(initStage0ScriptTemplate)
			},
		},
		{
			name: "selftest stage0 script",
			path: "examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/decomk-stage0.sh",
			mode: 0o755,
			render: func() ([]byte, error) {
				return stage0.RenderStage0Script(initStage0ScriptTemplate)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want, err := tc.render()
			if err != nil {
				t.Fatalf("render: %v", err)
			}

			path := repoPathFromCmdDecomk(tc.path)
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", path, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s is out of sync with templates; run `go generate ./...`", tc.path)
			}

			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("Stat(%s): %v", path, err)
			}
			if info.Mode().Perm() != tc.mode {
				t.Fatalf("%s mode: got %#o want %#o", tc.path, info.Mode().Perm(), tc.mode)
			}
		})
	}
}
