package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stevegt/decomk/confrepo"
	"github.com/stevegt/decomk/stage0"
)

func TestGeneratedConfrepoFilesMatchTemplates(t *testing.T) {
	t.Parallel()

	data := confrepo.ProducerDevcontainerData("decomk conf producer example").EnsureDefaults()

	// Intent: Enforce template/example parity for init `-conf` scaffolding so
	// checked-in confrepo examples cannot drift from embedded command templates.
	// Source: DI-013-20260424-190504 (TODO/013)
	tests := []struct {
		name   string
		path   string
		mode   os.FileMode
		render func() ([]byte, error)
	}{
		{
			name: "confrepo decomk.conf",
			path: "examples/confrepo/decomk.conf",
			mode: 0o644,
			render: func() ([]byte, error) {
				return stage0.RenderTemplate("confrepo.decomk.conf", initConfRepoDecomkConfTemplate, struct{}{})
			},
		},
		{
			name: "confrepo Makefile",
			path: "examples/confrepo/Makefile",
			mode: 0o644,
			render: func() ([]byte, error) {
				return stage0.RenderTemplate("confrepo.Makefile", initConfRepoMakefileTemplate, struct{}{})
			},
		},
		{
			name: "confrepo README",
			path: "examples/confrepo/README.md",
			mode: 0o644,
			render: func() ([]byte, error) {
				return stage0.RenderTemplate("confrepo.README.md", initConfRepoREADMETemplate, struct{}{})
			},
		},
		{
			name: "confrepo hello script",
			path: "examples/confrepo/bin/hello-world.sh",
			mode: 0o755,
			render: func() ([]byte, error) {
				return stage0.RenderTemplate("confrepo.hello-world.sh", initConfRepoHelloWorldTemplate, struct{}{})
			},
		},
		{
			name: "confrepo devcontainer json",
			path: "examples/confrepo/.devcontainer/devcontainer.json",
			mode: 0o644,
			render: func() ([]byte, error) {
				return stage0.RenderTemplate("confrepo.devcontainer.json", initConfRepoDevcontainerJSONTemplate, data)
			},
		},
		{
			name: "confrepo stage0 script",
			path: "examples/confrepo/.devcontainer/decomk-stage0.sh",
			mode: 0o755,
			render: func() ([]byte, error) {
				return stage0.RenderStage0Script(initStage0ScriptTemplate)
			},
		},
		{
			name: "confrepo Dockerfile",
			path: "examples/confrepo/.devcontainer/Dockerfile",
			mode: 0o644,
			render: func() ([]byte, error) {
				return stage0.RenderTemplate("confrepo.Dockerfile", initConfRepoDockerfileTemplate, data)
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

			path := filepath.Clean(filepath.Join("..", "..", tc.path))
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
