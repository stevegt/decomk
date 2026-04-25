// Package confrepo centralizes defaults for shared decomk config-repo
// scaffolding.
//
// Intent: Keep conf-repo initialization defaults in one package so command
// handlers, generators, and sync tests cannot drift on path/value assumptions.
// Source: DI-013-20260422-110500 (TODO/013)
package confrepo

import (
	"path/filepath"

	"github.com/stevegt/decomk/stage0"
)

const (
	// DefaultName is the default devcontainer name used for conf-repo producer
	// scaffolding when the caller does not provide one.
	DefaultName = "decomk conf producer"

	// DefaultConfURI is the placeholder URI scaffolded into conf-repo
	// devcontainer settings until operators replace it with a real repo URI.
	DefaultConfURI = "git:https://github.com/<org>/<conf-repo>.git?ref=main"

	// DefaultHome is the default DECOMK_HOME value scaffolded for producer
	// configs.
	DefaultHome = "/var/decomk"

	// DefaultLogDir is the default DECOMK_LOG_DIR value scaffolded for producer
	// configs.
	DefaultLogDir = "/var/log/decomk"
)

// ProducerDevcontainerData returns canonical starter data for the conf-repo
// producer devcontainer scaffold.
func ProducerDevcontainerData(name string) stage0.DevcontainerTemplateData {
	if name == "" {
		name = DefaultName
	}
	remoteIdentityUser := stage0.DefaultDevcontainerUser
	remoteIdentityUID := stage0.DefaultDevcontainerUID
	// Intent: Keep producer prebuild/runtime identity deterministic by pinning one
	// configured non-root user/UID and disabling runtime UID remap.
	// Source: DI-001-20260424-190437 (TODO/001)
	disableUIDRemap := false
	return stage0.DevcontainerTemplateData{
		Name:                 name,
		BuildDockerfile:      "Dockerfile",
		BuildContext:         "..",
		RemoteIdentityUser:   remoteIdentityUser,
		RemoteIdentityUID:    remoteIdentityUID,
		RemoteUser:           remoteIdentityUser,
		ContainerUser:        remoteIdentityUser,
		UpdateRemoteUserUID:  &disableUIDRemap,
		Home:                 DefaultHome,
		LogDir:               DefaultLogDir,
		ToolURI:              stage0.DefaultToolURI,
		ConfURI:              DefaultConfURI,
		FailNoBoot:           stage0.DefaultFailNoBoot,
		UpdateContentCommand: stage0.DefaultUpdateContentCommand,
		PostCreateCommand:    stage0.DefaultPostCreateCommand,
	}
}

// ProducerDevcontainerDataWithIdentity returns canonical starter data for the
// conf-repo producer devcontainer scaffold with explicit identity values.
func ProducerDevcontainerDataWithIdentity(name, remoteIdentityUser, remoteIdentityUID string) stage0.DevcontainerTemplateData {
	// Intent: Keep producer identity overrides centralized so both interactive
	// init paths and generators apply one consistent user/UID mapping contract.
	// Source: DI-001-20260424-190437 (TODO/001)
	data := ProducerDevcontainerData(name)
	if remoteIdentityUser != "" {
		data.RemoteIdentityUser = remoteIdentityUser
		data.RemoteUser = remoteIdentityUser
		data.ContainerUser = remoteIdentityUser
	}
	if remoteIdentityUID != "" {
		data.RemoteIdentityUID = remoteIdentityUID
	}
	return data
}

// ManagedPaths returns all conf-repo scaffold output paths relative to repo
// root.
func ManagedPaths() []string {
	return []string{
		"decomk.conf",
		"Makefile",
		"README.md",
		filepath.Join("bin", "hello-world.sh"),
		filepath.Join(".devcontainer", "devcontainer.json"),
		filepath.Join(".devcontainer", "decomk-stage0.sh"),
		filepath.Join(".devcontainer", "Dockerfile"),
	}
}
