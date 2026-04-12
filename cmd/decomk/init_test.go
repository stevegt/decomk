package main

import (
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
		Name:              `repo "alpha"`,
		ConfURI:           "git:https://example.com/conf.git?ref=prod",
		ToolURI:           "go:github.com/stevegt/decomk/cmd/decomk@stable",
		Home:              "/var/decomk",
		LogDir:            "/var/log/decomk",
		DecomkRunArgs:     "all INSTALL",
		PostCreateCommand: stage0.DefaultPostCreateCommand,
	}

	rendered, err := stage0.RenderDevcontainerJSON(initDevcontainerJSONTemplate, data)
	if err != nil {
		t.Fatalf("RenderDevcontainerJSON() error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(rendered, &decoded); err != nil {
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
		"DECOMK_HOME":     data.Home,
		"DECOMK_LOG_DIR":  data.LogDir,
		"DECOMK_TOOL_URI": data.ToolURI,
		"DECOMK_CONF_URI": data.ConfURI,
		"DECOMK_RUN_ARGS": data.DecomkRunArgs,
	}
	for key, want := range tests {
		if got := envMap[key]; got != want {
			t.Fatalf("%s: got %#v want %#v", key, got, want)
		}
	}
}

func TestWriteInitStage0_ForcePolicy(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	data := stage0.DevcontainerTemplateData{
		Name:              "repo",
		ConfURI:           "git:https://example.com/conf.git",
		ToolURI:           "go:github.com/stevegt/decomk/cmd/decomk@stable",
		Home:              "/var/decomk",
		LogDir:            "/var/log/decomk",
		DecomkRunArgs:     "all",
		PostCreateCommand: stage0.DefaultPostCreateCommand,
	}

	results, err := writeInitStage0(repoRoot, data, false)
	if err != nil {
		t.Fatalf("writeInitStage0() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("result count: got %d want 2", len(results))
	}

	devcontainerPath := filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(`{"broken":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile(devcontainerPath): %v", err)
	}

	if _, err := writeInitStage0(repoRoot, data, false); err == nil {
		t.Fatalf("writeInitStage0() error: got nil want conflict error")
	}
	if _, err := writeInitStage0(repoRoot, data, true); err != nil {
		t.Fatalf("writeInitStage0(force=true) error: %v", err)
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
		"-tool-uri", "go:github.com/stevegt/decomk/cmd/decomk@stable",
		"-home", "/var/decomk",
		"-log-dir", "/var/log/decomk",
		"-run-args", "all",
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
	if !strings.Contains(out, "devcontainer.json") || !strings.Contains(out, "postCreateCommand.sh") {
		t.Fatalf("stdout: got %q want both stage-0 paths", out)
	}

	devcontainerPath := filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")
	postCreatePath := filepath.Join(repoRoot, ".devcontainer", "postCreateCommand.sh")

	if _, err := os.Stat(devcontainerPath); err != nil {
		t.Fatalf("Stat(devcontainer.json): %v", err)
	}
	info, err := os.Stat(postCreatePath)
	if err != nil {
		t.Fatalf("Stat(postCreateCommand.sh): %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("postCreateCommand.sh should be executable; mode=%#o", info.Mode().Perm())
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
		"-tool-uri", "go:github.com/stevegt/decomk/cmd/decomk@stable",
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
