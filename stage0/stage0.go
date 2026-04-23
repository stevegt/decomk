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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const (
	// DefaultUpdateContentCommand is the lifecycle hook command used by generated
	// devcontainer.json files for prebuild/common work.
	DefaultUpdateContentCommand = "bash .devcontainer/decomk-stage0.sh updateContent"

	// DefaultPostCreateCommand is the lifecycle hook command used by generated
	// devcontainer.json files for runtime/user work.
	DefaultPostCreateCommand = "bash .devcontainer/decomk-stage0.sh postCreate"

	// DefaultToolURI is the canonical stage-0 tool source expression used when
	// no explicit DECOMK_TOOL_URI is provided in generated devcontainer files.
	//
	// Intent: Keep stage-0 bootstrap source selection on one explicit URI grammar
	// (`go:` / `git:`) so install-vs-clone behavior is determined by source value
	// instead of a parallel mode variable family.
	// Source: DI-001-20260412-170500 (TODO/001)
	DefaultToolURI = "go:github.com/stevegt/decomk/cmd/decomk@stable"
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
//   - Name, containerEnv, updateContentCommand, and postCreateCommand are always emitted.
type DevcontainerTemplateData struct {
	Name                 string
	BuildDockerfile      string
	BuildContext         string
	RunArgs              []string
	RemoteUser           string
	Home                 string
	LogDir               string
	ToolURI              string
	ConfURI              string
	UpdateContentCommand string
	PostCreateCommand    string
}

// EnsureDefaults populates standard defaults for fields that should always have
// a stable value unless explicitly overridden.
func (data DevcontainerTemplateData) EnsureDefaults() DevcontainerTemplateData {
	// Intent: Keep the generated lifecycle contract explicit and phase-aware by
	// default so `updateContent` and `postCreate` always call stage-0 with an
	// unambiguous phase argument.
	// Source: DI-001-20260416-223600 (TODO/001)
	if data.UpdateContentCommand == "" {
		data.UpdateContentCommand = DefaultUpdateContentCommand
	}
	if data.PostCreateCommand == "" {
		data.PostCreateCommand = DefaultPostCreateCommand
	}
	if data.ToolURI == "" {
		data.ToolURI = DefaultToolURI
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
		Name:                 "decomk (example; set DECOMK_CONF_URI)",
		BuildDockerfile:      "Dockerfile",
		BuildContext:         ".",
		RemoteUser:           "dev",
		Home:                 "/var/decomk",
		LogDir:               "/var/log/decomk",
		ToolURI:              DefaultToolURI,
		ConfURI:              "",
		UpdateContentCommand: DefaultUpdateContentCommand,
		PostCreateCommand:    DefaultPostCreateCommand,
	}
}

// SelftestDevcontainerData returns the canonical data profile for
// examples/decomk-selftest/devpod-local/workspace-template/.devcontainer/devcontainer.json.
func SelftestDevcontainerData() DevcontainerTemplateData {
	return DevcontainerTemplateData{
		Name:                 "decomk-selftest-devpod-local",
		BuildDockerfile:      "Dockerfile",
		BuildContext:         "..",
		RunArgs:              []string{"--add-host=host.docker.internal:host-gateway"},
		RemoteUser:           "dev",
		Home:                 "/tmp/decomk-selftest/home",
		LogDir:               "/tmp/decomk-selftest/log",
		ToolURI:              "__DECOMK_TOOL_URI__",
		ConfURI:              "__DECOMK_CONF_URI__",
		UpdateContentCommand: DefaultUpdateContentCommand,
		PostCreateCommand:    DefaultPostCreateCommand,
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

// RenderStage0Script renders the canonical stage-0 lifecycle script template.
func RenderStage0Script(templateSource string) ([]byte, error) {
	return RenderTemplate("decomk-stage0.sh", templateSource, struct{}{})
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
			// Intent: Never hide temp-file cleanup failures during atomic writes;
			// preserve all error causes for debuggable write failures.
			// Source: DI-008-20260412-122157 (TODO/008)
			if removeErr := os.Remove(tmpPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove temp file %s: %w", tmpPath, removeErr))
			}
		}
	}()

	if _, err = tmp.Write(content); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close temp file after write failure: %w", closeErr))
		}
		return err
	}
	if err = tmp.Chmod(mode); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close temp file after chmod failure: %w", closeErr))
		}
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
