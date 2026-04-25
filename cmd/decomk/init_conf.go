package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/stevegt/decomk/stage0"
)

// writeInitConfScaffold renders and writes all managed conf-repo scaffold
// files.
func writeInitConfScaffold(repoRoot string, data stage0.DevcontainerTemplateData, force bool) ([]initWriteResult, error) {
	devcontainerData := data.EnsureDefaults()

	decomkConf, err := stage0.RenderTemplate("confrepo.decomk.conf", initConfRepoDecomkConfTemplate, struct{}{})
	if err != nil {
		return nil, err
	}
	makefile, err := stage0.RenderTemplate("confrepo.Makefile", initConfRepoMakefileTemplate, struct{}{})
	if err != nil {
		return nil, err
	}
	readme, err := stage0.RenderTemplate("confrepo.README.md", initConfRepoREADMETemplate, struct{}{})
	if err != nil {
		return nil, err
	}
	helloScript, err := stage0.RenderTemplate("confrepo.hello-world.sh", initConfRepoHelloWorldTemplate, struct{}{})
	if err != nil {
		return nil, err
	}
	devcontainerJSON, err := stage0.RenderTemplate("confrepo.devcontainer.json", initConfRepoDevcontainerJSONTemplate, devcontainerData)
	if err != nil {
		return nil, err
	}
	stage0Script, err := stage0.RenderStage0Script(initStage0ScriptTemplate)
	if err != nil {
		return nil, err
	}
	dockerfile, err := stage0.RenderTemplate("confrepo.Dockerfile", initConfRepoDockerfileTemplate, devcontainerData)
	if err != nil {
		return nil, err
	}

	type outputFile struct {
		relPath string
		content []byte
		mode    os.FileMode
	}
	files := []outputFile{
		{relPath: "decomk.conf", content: decomkConf, mode: 0o644},
		{relPath: "Makefile", content: makefile, mode: 0o644},
		{relPath: "README.md", content: readme, mode: 0o644},
		{relPath: filepath.Join("bin", "hello-world.sh"), content: helloScript, mode: 0o755},
		{relPath: filepath.Join(".devcontainer", "devcontainer.json"), content: devcontainerJSON, mode: 0o644},
		{relPath: filepath.Join(".devcontainer", "decomk-stage0.sh"), content: stage0Script, mode: 0o755},
		{relPath: filepath.Join(".devcontainer", "Dockerfile"), content: dockerfile, mode: 0o644},
	}

	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, filepath.Join(repoRoot, file.relPath))
	}
	if !force {
		// Intent: Keep conf-repo initialization conservative by default and force
		// explicit operator acknowledgement before overwriting managed starter files.
		// Source: DI-013-20260424-190504 (TODO/013)
		existing, err := existingInitTargets(paths...)
		if err != nil {
			return nil, err
		}
		if len(existing) > 0 {
			return nil, initConfExistingTargetsError(existing)
		}
	}

	results := make([]initWriteResult, 0, len(files))
	for _, file := range files {
		absPath := filepath.Join(repoRoot, file.relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, err
		}
		status, err := writeInitFile(absPath, file.content, file.mode, force)
		if err != nil {
			return nil, err
		}
		results = append(results, initWriteResult{Path: absPath, Status: status})
	}

	return results, nil
}

// initConfExistingTargetsError formats strict non-force guidance for existing
// conf-repo scaffold files.
func initConfExistingTargetsError(existing []string) error {
	var builder strings.Builder
	builder.WriteString("refusing to overwrite existing conf-repo scaffold file(s) without -f/-force:\n")
	for _, path := range existing {
		builder.WriteString("  - ")
		builder.WriteString(path)
		builder.WriteString("\n")
	}
	builder.WriteString("recommended workflow:\n")
	builder.WriteString("  1) git commit the current files\n")
	builder.WriteString("  2) run decomk init -conf -f ...\n")
	builder.WriteString("  3) review and merge with git difftool\n")
	return errors.New(builder.String())
}
