package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stevegt/decomk/stage0"
)

func TestRenderInitTemplate_DevcontainerJSON(t *testing.T) {
	t.Parallel()

	data := stage0.DevcontainerTemplateData{
		Name:                 `repo "alpha"`,
		ConfURI:              "git:https://example.com/conf.git?ref=prod",
		ToolURI:              stage0.DefaultToolURI,
		Home:                 "/var/decomk",
		LogDir:               "/var/log/decomk",
		FailNoBoot:           "true",
		UpdateContentCommand: stage0.DefaultUpdateContentCommand,
		PostCreateCommand:    stage0.DefaultPostCreateCommand,
	}

	rendered, err := stage0.RenderDevcontainerJSON(initDevcontainerJSONTemplate, data)
	if err != nil {
		t.Fatalf("RenderDevcontainerJSON() error: %v", err)
	}

	var decoded map[string]any
	renderedWithoutComments, err := stripJSONCLineCommentsForInit(rendered)
	if err != nil {
		t.Fatalf("stripJSONCLineCommentsForInit() error: %v", err)
	}
	if err := json.Unmarshal(renderedWithoutComments, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if got, want := decoded["name"], data.Name; got != want {
		t.Fatalf("name: got %#v want %#v", got, want)
	}

	envMap, ok := decoded["containerEnv"].(map[string]any)
	if !ok {
		t.Fatalf("containerEnv: got %#v", decoded["containerEnv"])
	}

	tests := map[string]string{
		"DECOMK_HOME":        data.Home,
		"DECOMK_LOG_DIR":     data.LogDir,
		"DECOMK_TOOL_URI":    data.ToolURI,
		"DECOMK_CONF_URI":    data.ConfURI,
		"DECOMK_FAIL_NOBOOT": data.FailNoBoot,
	}
	for key, want := range tests {
		if got := envMap[key]; got != want {
			t.Fatalf("%s: got %#v want %#v", key, got, want)
		}
	}
	if got := decoded["updateContentCommand"]; got != data.UpdateContentCommand {
		t.Fatalf("updateContentCommand: got %#v want %#v", got, data.UpdateContentCommand)
	}
	if got := decoded["postCreateCommand"]; got != data.PostCreateCommand {
		t.Fatalf("postCreateCommand: got %#v want %#v", got, data.PostCreateCommand)
	}
}

func TestWriteInitStage0_ForcePolicy(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	data := initConsumerDevcontainerTemplateData{
		Name:  "repo",
		Image: "ghcr.io/example/repo:dev",
	}

	results, err := writeInitStage0(repoRoot, data, false)
	if err != nil {
		t.Fatalf("writeInitStage0() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("result count: got %d want 2", len(results))
	}

	if _, err := writeInitStage0(repoRoot, data, false); err == nil {
		t.Fatalf("writeInitStage0() error: got nil want existing-file error")
	} else {
		if !strings.Contains(err.Error(), "-f/-force") {
			t.Fatalf("error: got %q want force guidance", err.Error())
		}
		if !strings.Contains(err.Error(), "git difftool") {
			t.Fatalf("error: got %q want git difftool guidance", err.Error())
		}
	}

	devcontainerPath := filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(`{"broken":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile(devcontainerPath): %v", err)
	}

	if _, err := writeInitStage0(repoRoot, data, true); err != nil {
		t.Fatalf("writeInitStage0(force=true) error: %v", err)
	}
}

func TestWriteInitStage0_FailsIfEitherTargetExists(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	devcontainerDir := filepath.Join(repoRoot, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(devcontainerDir): %v", err)
	}

	jsonPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(jsonPath, []byte(`{"name":"existing"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(devcontainer.json): %v", err)
	}

	data := initConsumerDevcontainerTemplateData{
		Name:  "repo",
		Image: "ghcr.io/example/repo:dev",
	}

	if _, err := writeInitStage0(repoRoot, data, false); err == nil {
		t.Fatalf("writeInitStage0() error: got nil want existing-file error")
	}

	stage0Path := filepath.Join(devcontainerDir, "decomk-stage0.sh")
	if _, err := os.Stat(stage0Path); !os.IsNotExist(err) {
		t.Fatalf("decomk-stage0.sh should remain missing; err=%v", err)
	}
}

func TestCmdInit_OverwriteCheckRunsBeforeValidationAndPrompts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	devcontainerDir := filepath.Join(repoRoot, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(devcontainerDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"name":"existing"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(devcontainer.json): %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-repo-root", repoRoot,
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdInit() error: got nil want overwrite error")
	}
	if code != 1 {
		t.Fatalf("cmdInit() code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "refusing to overwrite existing stage-0 file(s)") {
		t.Fatalf("error: got %q want overwrite refusal", err.Error())
	}
	if strings.Contains(err.Error(), "requires -image") {
		t.Fatalf("error: got image-source failure %q; expected overwrite refusal first", err.Error())
	}
}

func TestValidateFailNoBootValue(t *testing.T) {
	t.Parallel()

	validValues := []string{"", "false", "true", "0", "1", "no", "yes", "off", "on", " TRUE "}
	for _, value := range validValues {
		value := value
		t.Run("valid_"+strings.ReplaceAll(strings.TrimSpace(strings.ToLower(value)), " ", "_"), func(t *testing.T) {
			t.Parallel()
			if err := validateFailNoBootValue(value); err != nil {
				t.Fatalf("validateFailNoBootValue(%q) error: %v", value, err)
			}
		})
	}

	if err := validateFailNoBootValue("maybe"); err == nil {
		t.Fatalf("validateFailNoBootValue(maybe): expected error, got nil")
	}
}

func TestCmdInit_ConsumerMisusedProducerFlagShowsUsage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-repo-root", t.TempDir(),
		"-conf-uri", "git:https://example.com/conf.git",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdInit() error: got nil want misuse error")
	}
	if code != 2 {
		t.Fatalf("cmdInit() code: got %d want 2", code)
	}
	if got, want := err.Error(), "-conf-uri is only valid with -conf"; !strings.Contains(got, want) {
		t.Fatalf("error: got %q want substring %q", got, want)
	}
	usage := stderr.String()
	if !strings.Contains(usage, "Mode note:") {
		t.Fatalf("usage: got %q want mode note", usage)
	}
	if !strings.Contains(usage, "image producer repo is usually the same") {
		t.Fatalf("usage: got %q want producer/conf relationship note", usage)
	}
}

func TestCmdInit_ConsumerMisusedProducerFlagReportsFirstOffender(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-repo-root", t.TempDir(),
		"-home", "/var/decomk",
		"-conf-uri", "git:https://example.com/conf.git",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdInit() error: got nil want misuse error")
	}
	if code != 2 {
		t.Fatalf("cmdInit() code: got %d want 2", code)
	}
	if got, want := err.Error(), "-conf-uri is only valid with -conf"; !strings.Contains(got, want) {
		t.Fatalf("error: got %q want first-flag substring %q", got, want)
	}
}

func TestCmdInit_HelpIncludesImageProducerConsumerUsageNote(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{"-h"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit() code: got %d want 0", code)
	}
	usage := stderr.String()
	if !strings.Contains(usage, "Image consumer mode is the default") {
		t.Fatalf("usage: got %q want image consumer note", usage)
	}
	if !strings.Contains(usage, "Image producer mode uses `decomk init -conf`") {
		t.Fatalf("usage: got %q want image producer note", usage)
	}
}

func TestCmdInit_NoPromptWritesFiles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := []string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-image", "ghcr.io/acme/custom-base:testing",
	}
	code, err := cmdInit(args, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit() code: got %d want 0", code)
	}

	if got := stderr.String(); got != "" {
		t.Fatalf("stderr: got %q want empty", got)
	}
	out := stdout.String()
	if !strings.Contains(out, "devcontainer.json") || !strings.Contains(out, "decomk-stage0.sh") {
		t.Fatalf("stdout: got %q want both stage-0 paths", out)
	}

	devcontainerPath := filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")
	stage0Path := filepath.Join(repoRoot, ".devcontainer", "decomk-stage0.sh")

	if _, err := os.Stat(devcontainerPath); err != nil {
		t.Fatalf("Stat(devcontainer.json): %v", err)
	}
	info, err := os.Stat(stage0Path)
	if err != nil {
		t.Fatalf("Stat(decomk-stage0.sh): %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("decomk-stage0.sh should be executable; mode=%#o", info.Mode().Perm())
	}

	decoded := decodeJSONCObjectFile(t, devcontainerPath)
	if got, want := decoded["image"], "ghcr.io/acme/custom-base:testing"; got != want {
		t.Fatalf("image: got %#v want %#v", got, want)
	}
	if got, want := decoded["name"], "myrepo"; got != want {
		t.Fatalf("name: got %#v want %#v", got, want)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded key count: got %d want 2 (name,image); decoded=%#v", len(decoded), decoded)
	}
}

func TestCmdInit_ForceNoPromptPreservesExistingImageDefaults(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	devcontainerDir := filepath.Join(repoRoot, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(devcontainerDir): %v", err)
	}
	existing := `{
  // existing defaults should be reused on -f reruns
  "name": "existing-repo",
  "image": "ghcr.io/acme/custom-base:testing"
}`
	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(devcontainer.json): %v", err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "decomk-stage0.sh"), []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(decomk-stage0.sh): %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-repo-root", repoRoot,
		"-no-prompt",
		"-f",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit() code: got %d want 0", code)
	}

	decoded := decodeJSONCObjectFile(t, devcontainerPath)
	if got, want := decoded["name"], "existing-repo"; got != want {
		t.Fatalf("name: got %#v want %#v", got, want)
	}
	if got, want := decoded["image"], "ghcr.io/acme/custom-base:testing"; got != want {
		t.Fatalf("image: got %#v want %#v", got, want)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded key count: got %d want 2 (name,image); decoded=%#v", len(decoded), decoded)
	}
}

func TestCmdInit_ForceNoPromptFailsWithoutImageSourceWhenExistingConfigHasNoImage(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	devcontainerDir := filepath.Join(repoRoot, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(devcontainerDir): %v", err)
	}
	existing := `{
  "name": "existing-build",
  "build": {
    "dockerfile": "Dockerfile.Block10",
    "context": "."
  }
}`
	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(devcontainer.json): %v", err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "decomk-stage0.sh"), []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(decomk-stage0.sh): %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-repo-root", repoRoot,
		"-no-prompt",
		"-f",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdInit() error: got nil want missing image source error")
	}
	if code != 1 {
		t.Fatalf("cmdInit() code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "requires -image or -conf-url") {
		t.Fatalf("error: got %q want image-source guidance", err.Error())
	}
}

func TestCmdInit_ForceAliasF(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	baseArgs := []string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-image", "ghcr.io/acme/base:dev",
	}

	code, err := cmdInit(baseArgs, &stdout, &stderr)
	if err != nil {
		t.Fatalf("first cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("first cmdInit() code: got %d want 0", code)
	}

	stdout.Reset()
	stderr.Reset()
	code, err = cmdInit(baseArgs, &stdout, &stderr)
	if err == nil {
		t.Fatalf("second cmdInit() error: got nil want existing-file error")
	}
	if code != 1 {
		t.Fatalf("second cmdInit() code: got %d want 1", code)
	}

	stdout.Reset()
	stderr.Reset()
	forceArgs := append([]string(nil), baseArgs...)
	forceArgs = append(forceArgs, "-f")
	code, err = cmdInit(forceArgs, &stdout, &stderr)
	if err != nil {
		t.Fatalf("forced cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("forced cmdInit() code: got %d want 0", code)
	}
}

func TestCmdInit_DefaultRepoRootUsesGitToplevel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	repoRoot := t.TempDir()
	if out, err := exec.Command("git", "-C", repoRoot, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, strings.TrimSpace(string(out)))
	}

	nested := filepath.Join(repoRoot, "nested", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll(nested): %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := os.Chdir(origWD); cleanupErr != nil {
			t.Errorf("cleanup Chdir(origWD): %v", cleanupErr)
		}
	})
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("Chdir(nested): %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-no-prompt",
		"-image", "ghcr.io/acme/base:dev",
		"-name", "myrepo",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit() code: got %d want 0", code)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")); err != nil {
		t.Fatalf("Stat(repoRoot devcontainer.json): %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, ".devcontainer", "devcontainer.json")); !os.IsNotExist(err) {
		t.Fatalf("nested .devcontainer should not exist; err=%v", err)
	}
}

func TestCmdInit_DefaultRepoRootErrorsOutsideGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	nonRepo := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := os.Chdir(origWD); cleanupErr != nil {
			t.Errorf("cleanup Chdir(origWD): %v", cleanupErr)
		}
	})
	if err := os.Chdir(nonRepo); err != nil {
		t.Fatalf("Chdir(nonRepo): %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-no-prompt",
		"-image", "ghcr.io/acme/base:dev",
		"-name", "myrepo",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdInit() error: got nil want error")
	}
	if code != 1 {
		t.Fatalf("cmdInit() code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "set -repo-root") {
		t.Fatalf("error: got %q want mention of -repo-root guidance", err.Error())
	}
}

func TestCmdInit_ProducerToolURIValidation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	confURI := "git:https://example.com/conf.git"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// go: URIs are valid for tool bootstrap.
	code, err := cmdInit([]string{
		"-conf",
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-conf-uri", confURI,
		"-tool-uri", stage0.DefaultToolURI,
		"-image", "mcr.microsoft.com/devcontainers/base:ubuntu-24.04",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("go URI cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("go URI cmdInit() code: got %d want 0", code)
	}
	decoded := decodeJSONCObjectFile(t, filepath.Join(repoRoot, ".devcontainer", "devcontainer.json"))
	containerEnv, ok := decoded["containerEnv"].(map[string]any)
	if !ok {
		t.Fatalf("containerEnv: got %#v want object", decoded["containerEnv"])
	}
	if got, want := containerEnv["DECOMK_TOOL_URI"], stage0.DefaultToolURI; got != want {
		t.Fatalf("DECOMK_TOOL_URI: got %#v want %#v (CLI override must beat producer defaults)", got, want)
	}

	// Invalid schemes should fail fast.
	stdout.Reset()
	stderr.Reset()
	code, err = cmdInit([]string{
		"-conf",
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-conf-uri", confURI,
		"-tool-uri", "zip:https://example.com/tool.zip",
		"-force",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("invalid tool URI cmdInit() error: got nil want error")
	}
	if code != 1 {
		t.Fatalf("invalid tool URI cmdInit() code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "must start with go: or git:") {
		t.Fatalf("error: got %q want tool URI scheme error", err.Error())
	}
}

func TestCmdInit_FailsWhenConfURLDerivationFailsInNoPromptMode(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-conf-url", "https://example.invalid/definitely-missing-conf.git",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdInit() error: got nil want conf-url derivation error")
	}
	if code != 1 {
		t.Fatalf("cmdInit() code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "derive image from -conf-url") {
		t.Fatalf("error: got %q want conf-url derivation context", err.Error())
	}
}

func TestCmdInit_NoPromptFailsWithoutImageSources(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdInit() error: got nil want missing-source error")
	}
	if code != 1 {
		t.Fatalf("cmdInit() code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "requires -image or -conf-url") {
		t.Fatalf("error: got %q want image-source guidance", err.Error())
	}
}

func TestCmdInit_NoPromptDerivesImageFromConfURL(t *testing.T) {
	t.Parallel()

	confURL := createTestProducerConfURL(t, "ghcr.io/acme/producer:block10")
	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-conf-url", confURL,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit() code: got %d want 0", code)
	}
	decoded := decodeJSONCObjectFile(t, filepath.Join(repoRoot, ".devcontainer", "devcontainer.json"))
	if got, want := decoded["name"], "myrepo"; got != want {
		t.Fatalf("name: got %#v want %#v", got, want)
	}
	if got, want := decoded["image"], "ghcr.io/acme/producer:block10"; got != want {
		t.Fatalf("image: got %#v want %#v", got, want)
	}
}

func TestCmdInit_ImageShortCircuitsConfURL(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-image", "ghcr.io/acme/explicit:stable",
		"-conf-url", "https://example.invalid/definitely-missing-conf.git",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit() code: got %d want 0", code)
	}
	decoded := decodeJSONCObjectFile(t, filepath.Join(repoRoot, ".devcontainer", "devcontainer.json"))
	if got, want := decoded["image"], "ghcr.io/acme/explicit:stable"; got != want {
		t.Fatalf("image: got %#v want %#v", got, want)
	}
}

func TestParseHTTPSourceURL_ValidationAndRef(t *testing.T) {
	t.Parallel()

	repoURL, gitRef, err := parseHTTPSourceURL("https://github.com/acme/decomk-conf.git?ref=main")
	if err != nil {
		t.Fatalf("parseHTTPSourceURL() error: %v", err)
	}
	if got, want := repoURL, "https://github.com/acme/decomk-conf.git"; got != want {
		t.Fatalf("repoURL: got %q want %q", got, want)
	}
	if got, want := gitRef, "main"; got != want {
		t.Fatalf("gitRef: got %q want %q", got, want)
	}

	if _, _, err := parseHTTPSourceURL("git@github.com:acme/decomk-conf.git"); err == nil {
		t.Fatalf("parseHTTPSourceURL(ssh) error: got nil want error")
	}
}

func TestStripJSONCLineCommentsForInit_InlineAndStringSlashSlash(t *testing.T) {
	t.Parallel()

	input := []byte(`{
  // full-line comment
  "image": "ghcr.io/acme/base:latest", // inline comment
  "note": "https://example.com/path//kept"
}`)
	stripped, err := stripJSONCLineCommentsForInit(input)
	if err != nil {
		t.Fatalf("stripJSONCLineCommentsForInit() error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(stripped, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v\nstripped:\n%s", err, string(stripped))
	}
	if got, want := decoded["image"], "ghcr.io/acme/base:latest"; got != want {
		t.Fatalf("image: got %#v want %#v", got, want)
	}
	if got, want := decoded["note"], "https://example.com/path//kept"; got != want {
		t.Fatalf("note: got %#v want %#v", got, want)
	}
}

func TestPromptConsumerInitImageSource_Option3KeepsExistingImage(t *testing.T) {
	t.Parallel()

	input := bytes.NewBufferString("3\n")
	reader := bufio.NewReader(input)
	var out bytes.Buffer
	image, err := promptConsumerInitImageSource(reader, &out, "ghcr.io/acme/existing:stable", "")
	if err != nil {
		t.Fatalf("promptConsumerInitImageSource() error: %v", err)
	}
	if got, want := image, "ghcr.io/acme/existing:stable"; got != want {
		t.Fatalf("image: got %q want %q", got, want)
	}
}

func TestPromptConsumerInitImageSource_Option3UnavailableWithoutExistingImage(t *testing.T) {
	t.Parallel()

	input := bytes.NewBufferString("3\n2\nghcr.io/acme/manual:dev\n")
	reader := bufio.NewReader(input)
	var out bytes.Buffer
	image, err := promptConsumerInitImageSource(reader, &out, "", "")
	if err != nil {
		t.Fatalf("promptConsumerInitImageSource() error: %v", err)
	}
	if got, want := image, "ghcr.io/acme/manual:dev"; got != want {
		t.Fatalf("image: got %q want %q", got, want)
	}
	if got := out.String(); !strings.Contains(got, "option 3 is available only when an existing image is present") {
		t.Fatalf("output: got %q want option-3 warning", got)
	}
}

func TestPromptConsumerInitImageSource_DerivationFailureFallsBackToManualPrompt(t *testing.T) {
	t.Parallel()

	input := bytes.NewBufferString("1\nhttps://example.invalid/definitely-missing-conf.git\nghcr.io/acme/manual:fallback\n")
	reader := bufio.NewReader(input)
	var out bytes.Buffer
	image, err := promptConsumerInitImageSource(reader, &out, "", "")
	if err != nil {
		t.Fatalf("promptConsumerInitImageSource() error: %v", err)
	}
	if got, want := image, "ghcr.io/acme/manual:fallback"; got != want {
		t.Fatalf("image: got %q want %q", got, want)
	}
	if got := out.String(); !strings.Contains(got, "warning: derive image from") {
		t.Fatalf("output: got %q want derivation warning", got)
	}
}

func TestWriteFileAtomic_PreservesFileOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "example.txt")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("WriteFile(original): %v", err)
	}

	// Force writeFileAtomic to fail during temp-file creation by removing parent.
	missingPath := filepath.Join(dir, "missing", "example.txt")
	err := stage0.WriteFileAtomic(missingPath, []byte("new"), 0o644)
	if err == nil {
		t.Fatalf("WriteFileAtomic() error: got nil want error")
	}

	// Existing file remains unchanged.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(path): %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("existing file changed unexpectedly: got %q want %q", string(got), "original")
	}
}

func createTestProducerConfURL(t *testing.T, image string) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(workDir, ".devcontainer"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workDir/.devcontainer): %v", err)
	}

	devcontainerJSON := `{
  "name": "test producer conf",
  "image": "__IMAGE__"
}`
	devcontainerJSON = strings.ReplaceAll(devcontainerJSON, "__IMAGE__", image)
	if err := os.WriteFile(filepath.Join(workDir, ".devcontainer", "devcontainer.json"), []byte(devcontainerJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(test producer devcontainer.json): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("test conf repo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(test producer README.md): %v", err)
	}

	if output, err := exec.Command("git", "-C", workDir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("git", "-C", workDir, "config", "user.email", "test@example.invalid").CombinedOutput(); err != nil {
		t.Fatalf("git config user.email: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("git", "-C", workDir, "config", "user.name", "decomk init tests").CombinedOutput(); err != nil {
		t.Fatalf("git config user.name: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("git", "-C", workDir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("git", "-C", workDir, "commit", "-q", "-m", "seed test producer conf").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, strings.TrimSpace(string(output)))
	}

	bareDir := filepath.Join(root, "confrepo.git")
	if output, err := exec.Command("git", "clone", "-q", "--bare", workDir, bareDir).CombinedOutput(); err != nil {
		t.Fatalf("git clone --bare: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("git", "--git-dir", bareDir, "update-server-info").CombinedOutput(); err != nil {
		t.Fatalf("git update-server-info: %v: %s", err, strings.TrimSpace(string(output)))
	}

	server := httptest.NewServer(http.FileServer(http.Dir(root)))
	t.Cleanup(server.Close)
	return server.URL + "/confrepo.git"
}

func decodeJSONCObjectFile(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	withoutComments, err := stripJSONCLineCommentsForInit(content)
	if err != nil {
		t.Fatalf("stripJSONCLineCommentsForInit(%s): %v", path, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(withoutComments, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", path, err)
	}
	return decoded
}

func stripJSONCLineComments(content []byte) ([]byte, error) {
	return stripJSONCLineCommentsForInit(content)
}
