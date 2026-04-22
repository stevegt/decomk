package main

import _ "embed"

var (
	// initConfRepoDecomkConfTemplate is the conf-repo template for decomk.conf.
	//
	//go:embed templates/confrepo.decomk.conf.tmpl
	initConfRepoDecomkConfTemplate string

	// initConfRepoMakefileTemplate is the conf-repo template for Makefile.
	//
	//go:embed templates/confrepo.Makefile.tmpl
	initConfRepoMakefileTemplate string

	// initConfRepoREADMETemplate is the conf-repo template for README.md.
	//
	//go:embed templates/confrepo.README.md.tmpl
	initConfRepoREADMETemplate string

	// initConfRepoHelloWorldTemplate is the conf-repo template for
	// bin/hello-world.sh.
	//
	//go:embed templates/confrepo.hello-world.sh.tmpl
	initConfRepoHelloWorldTemplate string

	// initConfRepoDevcontainerJSONTemplate is the conf-repo template for
	// .devcontainer/devcontainer.json.
	//
	//go:embed templates/confrepo.devcontainer.json.tmpl
	initConfRepoDevcontainerJSONTemplate string

	// initConfRepoDockerfileTemplate is the conf-repo template for
	// .devcontainer/Dockerfile.
	//
	//go:embed templates/confrepo.Dockerfile.tmpl
	initConfRepoDockerfileTemplate string
)

