// Command confrepogen renders canonical shared conf-repo starter files from
// decomk's embedded `init -conf` templates.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stevegt/decomk/confrepo"
	"github.com/stevegt/decomk/stage0"
)

type outputSpec struct {
	Path string
	Mode os.FileMode
	Kind string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("confrepogen", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "repo root containing cmd/decomk/templates")
	checkOnly := fs.Bool("check", false, "check generated files for drift instead of writing")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRoot, err := filepath.Abs(*repoRoot)
	if err != nil {
		return fmt.Errorf("abs repo root %q: %w", *repoRoot, err)
	}

	templateDir := filepath.Join(absRoot, "cmd", "decomk", "templates")
	decomkConfTemplate, err := os.ReadFile(filepath.Join(templateDir, "confrepo.decomk.conf.tmpl"))
	if err != nil {
		return err
	}
	makefileTemplate, err := os.ReadFile(filepath.Join(templateDir, "confrepo.Makefile.tmpl"))
	if err != nil {
		return err
	}
	readmeTemplate, err := os.ReadFile(filepath.Join(templateDir, "confrepo.README.md.tmpl"))
	if err != nil {
		return err
	}
	helloTemplate, err := os.ReadFile(filepath.Join(templateDir, "confrepo.hello-world.sh.tmpl"))
	if err != nil {
		return err
	}
	devcontainerTemplate, err := os.ReadFile(filepath.Join(templateDir, "confrepo.devcontainer.json.tmpl"))
	if err != nil {
		return err
	}
	dockerfileTemplate, err := os.ReadFile(filepath.Join(templateDir, "confrepo.Dockerfile.tmpl"))
	if err != nil {
		return err
	}
	stage0ScriptTemplate, err := os.ReadFile(filepath.Join(templateDir, "decomk-stage0.sh.tmpl"))
	if err != nil {
		return err
	}

	// Intent: Keep checked-in confrepo examples generated from the same template
	// contract used by `decomk init -conf` so docs/examples cannot drift from
	// command output.
	// Source: DI-013-20260424-190504 (TODO/013)
	targets := []outputSpec{
		{Path: filepath.Join(absRoot, "examples", "confrepo", "decomk.conf"), Mode: 0o644, Kind: "decomk.conf"},
		{Path: filepath.Join(absRoot, "examples", "confrepo", "Makefile"), Mode: 0o644, Kind: "Makefile"},
		{Path: filepath.Join(absRoot, "examples", "confrepo", "README.md"), Mode: 0o644, Kind: "README.md"},
		{Path: filepath.Join(absRoot, "examples", "confrepo", "bin", "hello-world.sh"), Mode: 0o755, Kind: "hello.sh"},
		{Path: filepath.Join(absRoot, "examples", "confrepo", ".devcontainer", "devcontainer.json"), Mode: 0o644, Kind: "devcontainer.json"},
		{Path: filepath.Join(absRoot, "examples", "confrepo", ".devcontainer", "decomk-stage0.sh"), Mode: 0o755, Kind: "decomk-stage0.sh"},
		{Path: filepath.Join(absRoot, "examples", "confrepo", ".devcontainer", "Dockerfile"), Mode: 0o644, Kind: "Dockerfile"},
	}

	data := confrepo.ProducerDevcontainerData("decomk conf producer example")

	var mismatches []string
	for _, target := range targets {
		var rendered []byte
		switch target.Kind {
		case "decomk.conf":
			rendered, err = stage0.RenderTemplate("confrepo.decomk.conf", string(decomkConfTemplate), struct{}{})
		case "Makefile":
			rendered, err = stage0.RenderTemplate("confrepo.Makefile", string(makefileTemplate), struct{}{})
		case "README.md":
			rendered, err = stage0.RenderTemplate("confrepo.README.md", string(readmeTemplate), struct{}{})
		case "hello.sh":
			rendered, err = stage0.RenderTemplate("confrepo.hello-world.sh", string(helloTemplate), struct{}{})
		case "devcontainer.json":
			rendered, err = stage0.RenderTemplate("confrepo.devcontainer.json", string(devcontainerTemplate), data.EnsureDefaults())
		case "decomk-stage0.sh":
			rendered, err = stage0.RenderStage0Script(string(stage0ScriptTemplate))
		case "Dockerfile":
			rendered, err = stage0.RenderTemplate("confrepo.Dockerfile", string(dockerfileTemplate), data.EnsureDefaults())
		default:
			return fmt.Errorf("unknown output kind %q for %s", target.Kind, target.Path)
		}
		if err != nil {
			return fmt.Errorf("render %s: %w", target.Path, err)
		}

		if *checkOnly {
			ok, compareErr := fileMatches(target.Path, rendered, target.Mode)
			if compareErr != nil {
				return compareErr
			}
			if !ok {
				mismatches = append(mismatches, target.Path)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target.Path), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(target.Path), err)
		}
		if err := stage0.WriteFileAtomic(target.Path, rendered, target.Mode); err != nil {
			return fmt.Errorf("write %s: %w", target.Path, err)
		}
	}

	if *checkOnly && len(mismatches) > 0 {
		return fmt.Errorf("generated confrepo files are out of date:\n%s\nrun: go generate ./...", strings.Join(mismatches, "\n"))
	}

	return nil
}

func fileMatches(path string, content []byte, mode os.FileMode) (bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if !bytes.Equal(existing, content) {
		return false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode().Perm() != mode {
		return false, nil
	}
	return true, nil
}
