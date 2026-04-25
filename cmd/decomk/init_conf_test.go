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

func TestCmdInitConf_WritesStarterTree(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code, err := cmdInit([]string{
		"-conf",
		"-repo-root", repoRoot,
		"-name", "conf producer",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit(-conf) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit(-conf) code: got %d want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr: got %q want empty", stderr.String())
	}

	paths := []struct {
		relPath string
		mode    os.FileMode
	}{
		{relPath: "decomk.conf", mode: 0o644},
		{relPath: "Makefile", mode: 0o644},
		{relPath: "README.md", mode: 0o644},
		{relPath: filepath.Join("bin", "hello-world.sh"), mode: 0o755},
		{relPath: filepath.Join(".devcontainer", "devcontainer.json"), mode: 0o644},
		{relPath: filepath.Join(".devcontainer", "decomk-stage0.sh"), mode: 0o755},
		{relPath: filepath.Join(".devcontainer", "Dockerfile"), mode: 0o644},
	}
	for _, tc := range paths {
		full := filepath.Join(repoRoot, tc.relPath)
		info, statErr := os.Stat(full)
		if statErr != nil {
			t.Fatalf("Stat(%s): %v", full, statErr)
		}
		if info.Mode().Perm() != tc.mode {
			t.Fatalf("%s mode: got %#o want %#o", tc.relPath, info.Mode().Perm(), tc.mode)
		}
	}

	devcontainerPath := filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")
	contents, err := os.ReadFile(devcontainerPath)
	if err != nil {
		t.Fatalf("ReadFile(devcontainer.json): %v", err)
	}
	jsonWithoutComments, err := stripJSONCLineComments(contents)
	if err != nil {
		t.Fatalf("stripJSONCLineComments(devcontainer.json): %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonWithoutComments, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(devcontainer.json): %v", err)
	}
	containerEnv, ok := decoded["containerEnv"].(map[string]any)
	if !ok {
		t.Fatalf("containerEnv: got %#v", decoded["containerEnv"])
	}
	if got, want := containerEnv["DECOMK_FAIL_NOBOOT"], stage0.DefaultFailNoBoot; got != want {
		t.Fatalf("DECOMK_FAIL_NOBOOT: got %#v want %#v", got, want)
	}
	if got, ok := containerEnv["DECOMK_REMOTE_USER"]; ok {
		t.Fatalf("DECOMK_REMOTE_USER: got %#v want omitted", got)
	}
	if got, ok := containerEnv["DECOMK_REMOTE_UID"]; ok {
		t.Fatalf("DECOMK_REMOTE_UID: got %#v want omitted", got)
	}
	if got, ok := decoded["remoteUser"]; ok {
		t.Fatalf("remoteUser: got %#v want omitted", got)
	}
	if got, ok := decoded["containerUser"]; ok {
		t.Fatalf("containerUser: got %#v want omitted", got)
	}
	if got, want := decoded["updateRemoteUserUID"], false; got != want {
		t.Fatalf("updateRemoteUserUID: got %#v want %#v", got, want)
	}

	dockerfilePath := filepath.Join(repoRoot, ".devcontainer", "Dockerfile")
	dockerfileContent, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile(Dockerfile): %v", err)
	}
	if !strings.Contains(string(dockerfileContent), "ENV DECOMK_REMOTE_USER="+stage0.DefaultDevcontainerUser) {
		t.Fatalf("Dockerfile missing DECOMK_REMOTE_USER ENV line:\n%s", string(dockerfileContent))
	}
	if !strings.Contains(string(dockerfileContent), "ENV DECOMK_REMOTE_UID="+stage0.DefaultDevcontainerUID) {
		t.Fatalf("Dockerfile missing DECOMK_REMOTE_UID ENV line:\n%s", string(dockerfileContent))
	}
}

func TestCmdInitConf_ForcePolicy(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	baseArgs := []string{"-conf", "-repo-root", repoRoot}
	code, err := cmdInit(baseArgs, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("first cmdInit(-conf) got code=%d err=%v", code, err)
	}

	stdout.Reset()
	stderr.Reset()
	code, err = cmdInit(baseArgs, &stdout, &stderr)
	if err == nil {
		t.Fatalf("second cmdInit(-conf) error: got nil want existing-file error")
	}
	if code != 1 {
		t.Fatalf("second cmdInit(-conf) code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "-f/-force") {
		t.Fatalf("error: got %q want force guidance", err.Error())
	}
	if !strings.Contains(err.Error(), "git difftool") {
		t.Fatalf("error: got %q want difftool guidance", err.Error())
	}

	stdout.Reset()
	stderr.Reset()
	code, err = cmdInit([]string{"-conf", "-repo-root", repoRoot, "-f"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("forced cmdInit(-conf) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("forced cmdInit(-conf) code: got %d want 0", code)
	}
}

func TestCmdInitConf_DefaultRepoRootUsesGitToplevel(t *testing.T) {
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
	code, err := cmdInit([]string{"-conf"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit(-conf) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit(-conf) code: got %d want 0", code)
	}

	if _, statErr := os.Stat(filepath.Join(repoRoot, "decomk.conf")); statErr != nil {
		t.Fatalf("Stat(repoRoot decomk.conf): %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(nested, "decomk.conf")); !os.IsNotExist(statErr) {
		t.Fatalf("nested decomk.conf should not exist; err=%v", statErr)
	}
	decoded := decodeJSONCObjectFile(t, filepath.Join(repoRoot, ".devcontainer", "devcontainer.json"))
	if got, want := decoded["name"], filepath.Base(repoRoot); got != want {
		t.Fatalf("name: got %#v want %#v", got, want)
	}
}

func TestCmdInitConf_NoPromptDefaultsNameToRepoBasename(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdInit([]string{"-conf", "-no-prompt", "-repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdInit(-conf -no-prompt) error: %v", err)
	}
	if code != 0 {
		t.Fatalf("cmdInit(-conf -no-prompt) code: got %d want 0", code)
	}
	decoded := decodeJSONCObjectFile(t, filepath.Join(repoRoot, ".devcontainer", "devcontainer.json"))
	if got, want := decoded["name"], filepath.Base(repoRoot); got != want {
		t.Fatalf("name: got %#v want %#v", got, want)
	}
}

func TestCmdInitConf_DefaultRepoRootErrorsOutsideGitRepo(t *testing.T) {
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
	code, err := cmdInit([]string{"-conf"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdInit(-conf) error: got nil want error")
	}
	if code != 1 {
		t.Fatalf("cmdInit(-conf) code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "set -repo-root") {
		t.Fatalf("error: got %q want mention of -repo-root guidance", err.Error())
	}
}
