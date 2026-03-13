// Package stage0 renders and writes devcontainer stage-0 bootstrap assets.
//
// It centralizes the shared template contract used by:
//   - `decomk init`
//   - generated example devcontainer files
//   - drift/parity tests
//
// Intent: Keep all stage-0 rendering paths on one data/templating contract so
// generated examples and init output cannot drift.
// Source: DI-001-20260312-141200 (TODO/001)
package stage0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const (
	// DefaultPostCreateCommand is the lifecycle hook command used by generated
	// devcontainer.json files.
	DefaultPostCreateCommand = "bash .devcontainer/postCreateCommand.sh"

	// DefaultToolInstallPkg is the install-mode `go install` package@version.
	DefaultToolInstallPkg = "github.com/stevegt/decomk/cmd/decomk@latest"

	// DefaultToolRepo is the default git URL used in clone mode.
	DefaultToolRepo = "https://github.com/stevegt/decomk"
)

// DevcontainerTemplateData is the full data model for rendering
// cmd/decomk/templates/devcontainer.json.tmpl.
//
// Optional sections:
//   - BuildDockerfile/BuildContext: emit "build" only when BuildDockerfile is non-empty.
//   - RunArgs: emit "runArgs" only when non-empty.
//   - RemoteUser: emit "remoteUser" only when non-empty.
//
// Required sections:
//   - Name, containerEnv, and postCreateCommand are always emitted.
type DevcontainerTemplateData struct {
	Name              string
	BuildDockerfile   string
	BuildContext      string
	RunArgs           []string
	RemoteUser        string
	Home              string
	LogDir            string
	ToolMode          string
	ToolInstallPkg    string
	ToolRepo          string
	ConfRepo          string
	DecomkRunArgs     string
	PostCreateCommand string
}

// EnsureDefaults populates standard defaults for fields that should always have
// a stable value unless explicitly overridden.
func (data DevcontainerTemplateData) EnsureDefaults() DevcontainerTemplateData {
	if data.PostCreateCommand == "" {
		data.PostCreateCommand = DefaultPostCreateCommand
	}
	if data.ToolInstallPkg == "" {
		data.ToolInstallPkg = DefaultToolInstallPkg
	}
	if data.ToolRepo == "" {
		data.ToolRepo = DefaultToolRepo
	}
	return data
}

// ProductionExampleDevcontainerData returns the canonical data profile for
// examples/devcontainer/devcontainer.json.
func ProductionExampleDevcontainerData() DevcontainerTemplateData {
	// Intent: Keep the checked-in production example standalone and runnable by
	// providing a local Dockerfile build and a non-root runtime user out of the
	// box.
	// Source: DI-001-20260313-183500 (TODO/001)
	return DevcontainerTemplateData{
		Name:              "decomk (example; set DECOMK_CONF_REPO)",
		BuildDockerfile:   "Dockerfile",
		BuildContext:      ".",
		RemoteUser:        "dev",
		Home:              "/var/decomk",
		LogDir:            "/var/log/decomk",
		ToolMode:          "install",
		ToolInstallPkg:    DefaultToolInstallPkg,
		ToolRepo:          DefaultToolRepo,
		ConfRepo:          "",
		DecomkRunArgs:     "all",
		PostCreateCommand: DefaultPostCreateCommand,
	}
}

// SelftestDevcontainerData returns the canonical data profile for
// examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json.
func SelftestDevcontainerData() DevcontainerTemplateData {
	return DevcontainerTemplateData{
		Name:              "decomk-selftest-devpod-local",
		BuildDockerfile:   "Dockerfile",
		BuildContext:      "..",
		RunArgs:           []string{"--add-host=host.docker.internal:host-gateway"},
		RemoteUser:        "dev",
		Home:              "/tmp/decomk-selftest/home",
		LogDir:            "/tmp/decomk-selftest/log",
		ToolMode:          "clone",
		ToolInstallPkg:    DefaultToolInstallPkg,
		ToolRepo:          "__DECOMK_TOOL_REPO__",
		ConfRepo:          "__DECOMK_CONF_REPO__",
		DecomkRunArgs:     "__DECOMK_RUN_ARGS__",
		PostCreateCommand: DefaultPostCreateCommand,
	}
}

// RenderTemplate renders one template source with data using a shared function
// map and deterministic missing-key behavior.
func RenderTemplate(name, source string, data any) ([]byte, error) {
	funcs := template.FuncMap{
		"json": func(value any) (string, error) {
			encoded, err := json.Marshal(value)
			if err != nil {
				return "", err
			}
			return string(encoded), nil
		},
	}

	tpl, err := template.New(name).Funcs(funcs).Option("missingkey=error").Parse(source)
	if err != nil {
		return nil, err
	}
	var rendered bytes.Buffer
	if err := tpl.Execute(&rendered, data); err != nil {
		return nil, err
	}
	return rendered.Bytes(), nil
}

// RenderDevcontainerJSON renders the canonical devcontainer template with the
// provided data.
func RenderDevcontainerJSON(templateSource string, data DevcontainerTemplateData) ([]byte, error) {
	data = data.EnsureDefaults()
	return RenderTemplate("devcontainer.json", templateSource, data)
}

// RenderPostCreateScript renders the canonical postCreate template.
func RenderPostCreateScript(templateSource string) ([]byte, error) {
	return RenderTemplate("postCreateCommand.sh", templateSource, struct{}{})
}

// WriteFileAtomic writes content to path using a same-directory temp file and
// atomic rename.
func WriteFileAtomic(path string, content []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

// EnsureMode applies mode to an existing path.
func EnsureMode(path string, mode os.FileMode) error {
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
