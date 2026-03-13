// Command stage0gen renders canonical devcontainer stage-0 files from
// decomk's embedded templates.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stevegt/decomk/stage0"
)

type outputSpec struct {
	Path string
	Mode os.FileMode
	Data stage0.DevcontainerTemplateData
	Kind string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("stage0gen", flag.ContinueOnError)
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

	devcontainerTemplatePath := filepath.Join(absRoot, "cmd", "decomk", "templates", "devcontainer.json.tmpl")
	postCreateTemplatePath := filepath.Join(absRoot, "cmd", "decomk", "templates", "postCreateCommand.sh.tmpl")
	devcontainerTemplate, err := os.ReadFile(devcontainerTemplatePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", devcontainerTemplatePath, err)
	}
	postCreateTemplate, err := os.ReadFile(postCreateTemplatePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", postCreateTemplatePath, err)
	}

	// Intent: Keep production and selftest stage-0 files generated from one canonical
	// template contract so drift is prevented by construction.
	// Source: DI-001-20260312-141200 (TODO/001)
	targets := []outputSpec{
		{
			Path: filepath.Join(absRoot, "examples", "devcontainer", "devcontainer.json"),
			Mode: 0o644,
			Data: stage0.ProductionExampleDevcontainerData(),
			Kind: "devcontainer",
		},
		{
			Path: filepath.Join(absRoot, "examples", "decomk-selftest", "devpod-local", "workspace-template", ".devcontainer", "devcontainer.json"),
			Mode: 0o644,
			Data: stage0.SelftestDevcontainerData(),
			Kind: "devcontainer",
		},
		{
			Path: filepath.Join(absRoot, "examples", "devcontainer", "postCreateCommand.sh"),
			Mode: 0o755,
			Kind: "postCreate",
		},
		{
			Path: filepath.Join(absRoot, "examples", "decomk-selftest", "devpod-local", "workspace-template", ".devcontainer", "postCreateCommand.sh"),
			Mode: 0o755,
			Kind: "postCreate",
		},
	}

	var mismatches []string
	for _, target := range targets {
		var rendered []byte
		switch target.Kind {
		case "devcontainer":
			rendered, err = stage0.RenderDevcontainerJSON(string(devcontainerTemplate), target.Data)
		case "postCreate":
			rendered, err = stage0.RenderPostCreateScript(string(postCreateTemplate))
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

		if err := stage0.WriteFileAtomic(target.Path, rendered, target.Mode); err != nil {
			return fmt.Errorf("write %s: %w", target.Path, err)
		}
	}

	if *checkOnly && len(mismatches) > 0 {
		return fmt.Errorf("generated stage-0 files are out of date:\n%s\nrun: go generate ./...", strings.Join(mismatches, "\n"))
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
