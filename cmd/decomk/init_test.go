package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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
	renderedWithoutComments, err := stripJSONCLineComments(rendered)
	if err != nil {
		t.Fatalf("stripJSONCLineComments() error: %v", err)
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
	data := stage0.DevcontainerTemplateData{
		Name:                 "repo",
		ConfURI:              "git:https://example.com/conf.git",
		ToolURI:              stage0.DefaultToolURI,
		Home:                 "/var/decomk",
		LogDir:               "/var/log/decomk",
		UpdateContentCommand: stage0.DefaultUpdateContentCommand,
		PostCreateCommand:    stage0.DefaultPostCreateCommand,
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

	data := stage0.DevcontainerTemplateData{
		Name:                 "repo",
		ConfURI:              "git:https://example.com/conf.git",
		ToolURI:              stage0.DefaultToolURI,
		Home:                 "/var/decomk",
		LogDir:               "/var/log/decomk",
		UpdateContentCommand: stage0.DefaultUpdateContentCommand,
		PostCreateCommand:    stage0.DefaultPostCreateCommand,
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
		"-tool-uri", "not-a-valid-uri",
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
	if strings.Contains(err.Error(), "DECOMK_TOOL_URI template value must start") {
		t.Fatalf("error: got validation failure %q; expected overwrite refusal first", err.Error())
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

func TestCmdInit_NoPromptWritesFiles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := []string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-conf-uri", "git:https://example.com/conf.git",
		"-tool-uri", stage0.DefaultToolURI,
		"-home", "/var/decomk",
		"-log-dir", "/var/log/decomk",
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
	if got, want := decoded["image"], stage0.DefaultDevcontainerImage; got != want {
		t.Fatalf("image: got %#v want %#v", got, want)
	}
	if _, ok := decoded["build"]; ok {
		t.Fatalf("build: got %#v want omitted for init defaults", decoded["build"])
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
  "image": "ghcr.io/acme/custom-base:testing",
  "containerEnv": {
    "DECOMK_HOME": "/var/custom/decomk",
    "DECOMK_LOG_DIR": "/var/custom/log",
    "DECOMK_TOOL_URI": "go:github.com/acme/decomk/cmd/decomk@latest",
    "DECOMK_CONF_URI": "git:https://example.com/conf.git",
    "DECOMK_FAIL_NOBOOT": "true"
  },
  "updateContentCommand": "bash .devcontainer/decomk-stage0.sh updateContent",
  "postCreateCommand": "bash .devcontainer/decomk-stage0.sh postCreate"
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
	envMap, ok := decoded["containerEnv"].(map[string]any)
	if !ok {
		t.Fatalf("containerEnv: got %#v", decoded["containerEnv"])
	}
	if got, want := envMap["DECOMK_HOME"], "/var/custom/decomk"; got != want {
		t.Fatalf("DECOMK_HOME: got %#v want %#v", got, want)
	}
	if got, want := envMap["DECOMK_LOG_DIR"], "/var/custom/log"; got != want {
		t.Fatalf("DECOMK_LOG_DIR: got %#v want %#v", got, want)
	}
	if got, want := envMap["DECOMK_TOOL_URI"], "go:github.com/acme/decomk/cmd/decomk@latest"; got != want {
		t.Fatalf("DECOMK_TOOL_URI: got %#v want %#v", got, want)
	}
	if got, want := envMap["DECOMK_CONF_URI"], "git:https://example.com/conf.git"; got != want {
		t.Fatalf("DECOMK_CONF_URI: got %#v want %#v", got, want)
	}
	if got, want := envMap["DECOMK_FAIL_NOBOOT"], "true"; got != want {
		t.Fatalf("DECOMK_FAIL_NOBOOT: got %#v want %#v", got, want)
	}
}

func TestCmdInit_ForceNoPromptPreservesExistingBuildDefaults(t *testing.T) {
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
  },
  "containerEnv": {
    "DECOMK_HOME": "/var/decomk",
    "DECOMK_LOG_DIR": "/var/log/decomk",
    "DECOMK_TOOL_URI": "go:github.com/stevegt/decomk/cmd/decomk@latest",
    "DECOMK_CONF_URI": "git:https://example.com/conf.git",
    "DECOMK_FAIL_NOBOOT": "false"
  },
  "updateContentCommand": "bash .devcontainer/decomk-stage0.sh updateContent",
  "postCreateCommand": "bash .devcontainer/decomk-stage0.sh postCreate"
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
	buildMap, ok := decoded["build"].(map[string]any)
	if !ok {
		t.Fatalf("build: got %#v want object", decoded["build"])
	}
	if got, want := buildMap["dockerfile"], "Dockerfile.Block10"; got != want {
		t.Fatalf("build.dockerfile: got %#v want %#v", got, want)
	}
	if got, want := buildMap["context"], "."; got != want {
		t.Fatalf("build.context: got %#v want %#v", got, want)
	}
	if _, ok := decoded["image"]; ok {
		t.Fatalf("image: got %#v want omitted when build is preserved", decoded["image"])
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
		"-conf-uri", "git:https://example.com/conf.git",
		"-tool-uri", stage0.DefaultToolURI,
		"-home", "/var/decomk",
		"-log-dir", "/var/log/decomk",
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
		"-conf-uri", "git:https://example.com/conf.git",
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
		"-conf-uri", "git:https://example.com/conf.git",
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

func TestCmdInit_ToolURIValidation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// go: URIs are valid for tool bootstrap.
	code, err := cmdInit([]string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-conf-uri", "git:https://example.com/conf.git",
		"-tool-uri", stage0.DefaultToolURI,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("go URI cmdInit() error: %v", err)
	}
	if code != 0 {
		t.Fatalf("go URI cmdInit() code: got %d want 0", code)
	}

	// Invalid schemes should fail fast.
	stdout.Reset()
	stderr.Reset()
	code, err = cmdInit([]string{
		"-no-prompt",
		"-repo-root", repoRoot,
		"-name", "myrepo",
		"-conf-uri", "git:https://example.com/conf.git",
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

func stripJSONCLineComments(content []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func decodeJSONCObjectFile(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	withoutComments, err := stripJSONCLineComments(content)
	if err != nil {
		t.Fatalf("stripJSONCLineComments(%s): %v", path, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(withoutComments, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", path, err)
	}
	return decoded
}
