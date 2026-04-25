package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type scriptedCommand struct {
	name   string
	args   []string
	output checkpointCommandOutput
	err    error
}

type scriptedCheckpointRunner struct {
	t     *testing.T
	steps []scriptedCommand
	index int
}

func (r *scriptedCheckpointRunner) run(name string, args ...string) (checkpointCommandOutput, error) {
	r.t.Helper()
	if r.index >= len(r.steps) {
		r.t.Fatalf("unexpected command call %q %q; no scripted steps left", name, strings.Join(args, " "))
	}
	step := r.steps[r.index]
	r.index++
	if step.name != name || !reflect.DeepEqual(step.args, args) {
		r.t.Fatalf(
			"unexpected command at step %d: got %q %q want %q %q",
			r.index,
			name,
			strings.Join(args, " "),
			step.name,
			strings.Join(step.args, " "),
		)
	}
	return step.output, step.err
}

func (r *scriptedCheckpointRunner) assertDone() {
	r.t.Helper()
	if r.index != len(r.steps) {
		r.t.Fatalf("not all scripted steps were consumed: used %d of %d", r.index, len(r.steps))
	}
}

func TestCmdCheckpointBuild_SuccessCleansContainerByDefault(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".devcontainer", "devcontainer.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	runner := &scriptedCheckpointRunner{
		t: t,
		steps: []scriptedCommand{
			{
				name: "devcontainer",
				args: []string{
					"up",
					"--workspace-folder", workspace,
					"--config", configPath,
					"--prebuild",
					"--log-level", checkpointBuildQuietLogLevel,
					"--log-format", "json",
				},
				output: checkpointCommandOutput{
					Stdout: `{"msg":"start"}` + "\n" + `{"containerId":"container-build-1"}` + "\n",
				},
			},
			{
				name: "docker",
				args: []string{"commit", "container-build-1", "registry.example/decomk:test-candidate"},
			},
			{
				name:   "docker",
				args:   []string{"image", "inspect", "--format", "{{.Id}}", "registry.example/decomk:test-candidate"},
				output: checkpointCommandOutput{Stdout: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"},
			},
			{
				name: "docker",
				args: []string{"rm", "-f", "container-build-1"},
			},
		},
	}

	deps := checkpointDeps{
		runner: runner,
		now: func() time.Time {
			return time.Date(2026, time.April, 20, 16, 0, 0, 0, time.UTC)
		},
		pid: func() int { return 4242 },
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdCheckpointWithDeps(
		[]string{
			"build",
			"-workspace-folder", workspace,
			"-config", ".devcontainer/devcontainer.json",
			"-tag", "registry.example/decomk:test-candidate",
			"-q",
		},
		&stdout,
		&stderr,
		deps,
	)
	if err != nil {
		t.Fatalf("cmdCheckpointWithDeps(build) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdCheckpointWithDeps(build) code: got %d want 0", code)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("stderr: got %q want empty", got)
	}

	var out checkpointBuildOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(build output): %v\nraw=%s", err, stdout.String())
	}
	if out.Command != checkpointSubcommandBuild {
		t.Fatalf("command: got %q want %q", out.Command, checkpointSubcommandBuild)
	}
	if out.ContainerID != "container-build-1" {
		t.Fatalf("containerID: got %q want %q", out.ContainerID, "container-build-1")
	}
	if out.SourceResolved == "" {
		t.Fatalf("sourceResolved should not be empty")
	}
	runner.assertDone()
}

func TestCmdCheckpointBuild_KeepContainerSkipsCleanup(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".devcontainer", "devcontainer.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	runner := &scriptedCheckpointRunner{
		t: t,
		steps: []scriptedCommand{
			{
				name: "devcontainer",
				args: []string{
					"up",
					"--workspace-folder", workspace,
					"--config", configPath,
					"--prebuild",
					"--log-level", checkpointBuildQuietLogLevel,
					"--log-format", "json",
				},
				output: checkpointCommandOutput{Stdout: `{"containerId":"container-keep"}` + "\n"},
			},
			{
				name: "docker",
				args: []string{"commit", "container-keep", "registry.example/decomk:keep-me"},
			},
			{
				name:   "docker",
				args:   []string{"image", "inspect", "--format", "{{.Id}}", "registry.example/decomk:keep-me"},
				output: checkpointCommandOutput{Stdout: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n"},
			},
		},
	}

	deps := checkpointDeps{runner: runner, now: time.Now, pid: os.Getpid}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdCheckpointWithDeps(
		[]string{
			"build",
			"-workspace-folder", workspace,
			"-config", ".devcontainer/devcontainer.json",
			"-tag", "registry.example/decomk:keep-me",
			"-keep-container",
			"-q",
		},
		&stdout,
		&stderr,
		deps,
	)
	if err != nil {
		t.Fatalf("cmdCheckpointWithDeps(build keep-container) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdCheckpointWithDeps(build keep-container) code: got %d want 0", code)
	}

	var out checkpointBuildOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(build keep output): %v", err)
	}
	if !out.KeepContainer {
		t.Fatalf("keepContainer: got false want true")
	}
	runner.assertDone()
}

func TestCmdCheckpointBuild_DefaultVerboseWritesLifecycleLogs(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".devcontainer", "devcontainer.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	runner := &scriptedCheckpointRunner{
		t: t,
		steps: []scriptedCommand{
			{
				name: "devcontainer",
				args: []string{
					"up",
					"--workspace-folder", workspace,
					"--config", configPath,
					"--prebuild",
					"--log-level", checkpointBuildVerboseLogLevel,
					"--log-format", "json",
				},
				output: checkpointCommandOutput{
					Stdout: `{"msg":"phase-start"}` + "\n" + `{"containerId":"container-verbose"}` + "\n",
					Stderr: "verbose-side-channel\n",
				},
			},
			{
				name: "docker",
				args: []string{"commit", "container-verbose", "registry.example/decomk:verbose"},
			},
			{
				name:   "docker",
				args:   []string{"image", "inspect", "--format", "{{.Id}}", "registry.example/decomk:verbose"},
				output: checkpointCommandOutput{Stdout: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n"},
			},
			{
				name: "docker",
				args: []string{"rm", "-f", "container-verbose"},
			},
		},
	}

	deps := checkpointDeps{runner: runner, now: time.Now, pid: os.Getpid}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdCheckpointWithDeps(
		[]string{
			"build",
			"-workspace-folder", workspace,
			"-config", ".devcontainer/devcontainer.json",
			"-tag", "registry.example/decomk:verbose",
		},
		&stdout,
		&stderr,
		deps,
	)
	if err != nil {
		t.Fatalf("cmdCheckpointWithDeps(build verbose default) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdCheckpointWithDeps(build verbose default) code: got %d want 0", code)
	}
	if got := stderr.String(); !strings.Contains(got, `{"containerId":"container-verbose"}`) {
		t.Fatalf("stderr missing devcontainer stdout logs: %q", got)
	}
	if got := stderr.String(); !strings.Contains(got, "verbose-side-channel") {
		t.Fatalf("stderr missing devcontainer stderr logs: %q", got)
	}

	runner.assertDone()
}

func TestCmdCheckpointPush_RequiresMoveForExistingDestination(t *testing.T) {
	t.Parallel()

	runner := &scriptedCheckpointRunner{
		t: t,
		steps: []scriptedCommand{
			{
				name:   "docker",
				args:   []string{"image", "inspect", "--format", "{{.Id}}", "registry.example/decomk:candidate"},
				output: checkpointCommandOutput{Stdout: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc\n"},
			},
			{
				name:   "docker",
				args:   []string{"manifest", "inspect", "registry.example/decomk:testing"},
				output: checkpointCommandOutput{Stdout: `{"schemaVersion":2}`},
			},
		},
	}

	deps := checkpointDeps{runner: runner, now: time.Now, pid: os.Getpid}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdCheckpointWithDeps(
		[]string{"push", "registry.example/decomk:candidate", "registry.example/decomk:testing"},
		&stdout,
		&stderr,
		deps,
	)
	if err == nil {
		t.Fatalf("cmdCheckpointWithDeps(push) error: got nil want error")
	}
	if code != 1 {
		t.Fatalf("cmdCheckpointWithDeps(push) code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "rerun with -m") {
		t.Fatalf("error: got %q want mention of -m", err.Error())
	}
	runner.assertDone()
}

func TestCmdCheckpointPush_MoveExistingDestination(t *testing.T) {
	t.Parallel()

	runner := &scriptedCheckpointRunner{
		t: t,
		steps: []scriptedCommand{
			{
				name:   "docker",
				args:   []string{"image", "inspect", "--format", "{{.Id}}", "registry.example/decomk:candidate"},
				output: checkpointCommandOutput{Stdout: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd\n"},
			},
			{
				name:   "docker",
				args:   []string{"manifest", "inspect", "registry.example/decomk:testing"},
				output: checkpointCommandOutput{Stdout: `{"schemaVersion":2}`},
			},
			{
				name: "docker",
				args: []string{"tag", "registry.example/decomk:candidate", "registry.example/decomk:testing"},
			},
			{
				name:   "docker",
				args:   []string{"push", "registry.example/decomk:testing"},
				output: checkpointCommandOutput{Stdout: "latest: digest: sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff size: 1234\n"},
			},
		},
	}

	deps := checkpointDeps{runner: runner, now: time.Now, pid: os.Getpid}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdCheckpointWithDeps(
		[]string{"push", "-m", "registry.example/decomk:candidate", "registry.example/decomk:testing"},
		&stdout,
		&stderr,
		deps,
	)
	if err != nil {
		t.Fatalf("cmdCheckpointWithDeps(push -m) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdCheckpointWithDeps(push -m) code: got %d want 0", code)
	}

	var out checkpointPublishOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(push output): %v", err)
	}
	if out.Command != checkpointSubcommandPush {
		t.Fatalf("command: got %q want %q", out.Command, checkpointSubcommandPush)
	}
	if !out.Move {
		t.Fatalf("move: got false want true")
	}
	if len(out.Tags) != 1 {
		t.Fatalf("tags length: got %d want 1", len(out.Tags))
	}
	if !out.Tags[0].Existed || !out.Tags[0].Moved {
		t.Fatalf("tag result: got %#v want existed=true moved=true", out.Tags[0])
	}
	if out.Tags[0].Status != "pushed" {
		t.Fatalf("tag status: got %q want %q", out.Tags[0].Status, "pushed")
	}
	if out.Tags[0].Digest == "" {
		t.Fatalf("tag digest should not be empty")
	}
	runner.assertDone()
}

func TestCmdCheckpointTag_ResolvesSourceWithPullFallback(t *testing.T) {
	t.Parallel()

	runner := &scriptedCheckpointRunner{
		t: t,
		steps: []scriptedCommand{
			{
				name:   "docker",
				args:   []string{"image", "inspect", "--format", "{{.Id}}", "registry.example/decomk@sha256:1111111111111111111111111111111111111111111111111111111111111111"},
				output: checkpointCommandOutput{Stderr: "Error: No such image"},
				err:    errors.New("exit status 1"),
			},
			{
				name:   "docker",
				args:   []string{"pull", "registry.example/decomk@sha256:1111111111111111111111111111111111111111111111111111111111111111"},
				output: checkpointCommandOutput{Stdout: "Pulled\n"},
			},
			{
				name:   "docker",
				args:   []string{"image", "inspect", "--format", "{{.Id}}", "registry.example/decomk@sha256:1111111111111111111111111111111111111111111111111111111111111111"},
				output: checkpointCommandOutput{Stdout: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n"},
			},
			{
				name:   "docker",
				args:   []string{"manifest", "inspect", "registry.example/decomk:stable"},
				output: checkpointCommandOutput{Stderr: "no such manifest"},
				err:    errors.New("exit status 1"),
			},
			{
				name: "docker",
				args: []string{"tag", "registry.example/decomk@sha256:1111111111111111111111111111111111111111111111111111111111111111", "registry.example/decomk:stable"},
			},
			{
				name:   "docker",
				args:   []string{"push", "registry.example/decomk:stable"},
				output: checkpointCommandOutput{Stdout: "stable: digest: sha256:abababababababababababababababababababababababababababababababab size: 1234\n"},
			},
		},
	}

	deps := checkpointDeps{runner: runner, now: time.Now, pid: os.Getpid}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdCheckpointWithDeps(
		[]string{
			"tag",
			"registry.example/decomk@sha256:1111111111111111111111111111111111111111111111111111111111111111",
			"registry.example/decomk:stable",
		},
		&stdout,
		&stderr,
		deps,
	)
	if err != nil {
		t.Fatalf("cmdCheckpointWithDeps(tag) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdCheckpointWithDeps(tag) code: got %d want 0", code)
	}

	var out checkpointPublishOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(tag output): %v", err)
	}
	if out.Command != checkpointSubcommandTag {
		t.Fatalf("command: got %q want %q", out.Command, checkpointSubcommandTag)
	}
	if !out.SourcePulled {
		t.Fatalf("sourcePulled: got false want true")
	}
	if len(out.Tags) != 1 {
		t.Fatalf("tags length: got %d want 1", len(out.Tags))
	}
	if out.Tags[0].Status != "retagged" {
		t.Fatalf("tag status: got %q want %q", out.Tags[0].Status, "retagged")
	}
	runner.assertDone()
}

func TestRunHelpIncludesCheckpointCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"decomk", "help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(help) code: got %d want 0", code)
	}
	if !strings.Contains(stdout.String(), "checkpoint") {
		t.Fatalf("help output missing checkpoint command: %q", stdout.String())
	}
}
