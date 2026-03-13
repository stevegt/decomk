package main

import _ "embed"

var (
	// initDevcontainerJSONTemplate is the stage-0 template for .devcontainer/devcontainer.json.
	//
	//go:embed templates/devcontainer.json.tmpl
	initDevcontainerJSONTemplate string

	// initPostCreateTemplate is the stage-0 template for .devcontainer/postCreateCommand.sh.
	//
	//go:embed templates/postCreateCommand.sh.tmpl
	initPostCreateTemplate string
)
