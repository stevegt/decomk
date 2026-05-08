package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdBranchRender_MainBuild(t *testing.T) {
	repoRoot := t.TempDir()
	writeBranchRegistryFixture(t, repoRoot, branchRegistryFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdBranch([]string{"render", "-repo-root", repoRoot, "-channel", "main"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdBranch(render main) error: %v\nstderr=%s", err, stderr.String())
	}
	if code != 0 {
		t.Fatalf("cmdBranch(render main) code: got %d want 0\nstderr=%s", code, stderr.String())
	}

	rendered := readRenderedDevcontainerFixture(t, repoRoot)
	build, ok := rendered["build"].(map[string]any)
	if !ok {
		t.Fatalf("rendered build: got %#v want object", rendered["build"])
	}
	if got, want := build["dockerfile"], "Dockerfile"; got != want {
		t.Fatalf("build.dockerfile: got %#v want %q", got, want)
	}
	if _, ok := rendered["image"]; ok {
		t.Fatalf("main render should not include image: %#v", rendered["image"])
	}
	env, ok := rendered["containerEnv"].(map[string]any)
	if !ok {
		t.Fatalf("containerEnv: got %#v want object", rendered["containerEnv"])
	}
	if got, want := env["DECOMK_TOOL_URI"], "go:github.com/stevegt/decomk/cmd/decomk@main"; got != want {
		t.Fatalf("DECOMK_TOOL_URI: got %#v want %q", got, want)
	}
	if _, ok := env["DECOMK_HOME"]; ok {
		t.Fatalf("render should not add image-owned DECOMK_HOME: %#v", env["DECOMK_HOME"])
	}
}

func TestCmdBranchRender_CheckFailsStale(t *testing.T) {
	repoRoot := t.TempDir()
	writeBranchRegistryFixture(t, repoRoot, branchRegistryFixture())
	writeFileFixture(t, filepath.Join(repoRoot, branchDevcontainerPath), []byte("{}\n"), 0o644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdBranch([]string{"render", "-repo-root", repoRoot, "-channel", "testing", "-check"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdBranch(render -check) error: got nil want stale error")
	}
	if code != 1 {
		t.Fatalf("cmdBranch(render -check) code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "is stale") {
		t.Fatalf("cmdBranch(render -check) error: got %q want stale message", err.Error())
	}
}

func TestCmdBranchRender_RejectsLatestForTesting(t *testing.T) {
	testCases := []struct {
		name        string
		channelName string
		oldRef      string
	}{
		{name: "testing", channelName: "testing", oldRef: "@testing"},
		{name: "stable", channelName: "stable", oldRef: "@stable"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			writeBranchRegistryFixture(t, repoRoot, strings.ReplaceAll(
				branchRegistryFixture(),
				"go:github.com/stevegt/decomk/cmd/decomk"+tc.oldRef,
				"go:github.com/stevegt/decomk/cmd/decomk@latest",
			))

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code, err := cmdBranch([]string{"render", "-repo-root", repoRoot, "-channel", tc.channelName}, &stdout, &stderr)
			if err == nil {
				t.Fatalf("cmdBranch(render %s) error: got nil want @latest rejection", tc.channelName)
			}
			if code != 1 {
				t.Fatalf("cmdBranch(render %s) code: got %d want 1", tc.channelName, code)
			}
			if !strings.Contains(err.Error(), "cannot use DECOMK_TOOL_URI ending in @latest") {
				t.Fatalf("cmdBranch(render %s) error: got %q want @latest rejection", tc.channelName, err.Error())
			}
		})
	}
}

func TestCmdBranchRender_AutoChannel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	repoRoot := t.TempDir()
	gitInit := exec.Command("git", "init", "-b", "testing", repoRoot)
	if output, err := gitInit.CombinedOutput(); err != nil {
		t.Skipf("git init -b testing failed, likely old git: %v\n%s", err, string(output))
	}
	writeBranchRegistryFixture(t, repoRoot, branchRegistryFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdBranch([]string{"render", "-repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdBranch(render auto) error: %v\nstderr=%s", err, stderr.String())
	}
	if code != 0 {
		t.Fatalf("cmdBranch(render auto) code: got %d want 0\nstderr=%s", code, stderr.String())
	}

	rendered := readRenderedDevcontainerFixture(t, repoRoot)
	if got, want := rendered["image"], "ghcr.io/ciwg/decomk-conf-cswg:testing"; got != want {
		t.Fatalf("image: got %#v want %q", got, want)
	}
}

func TestCmdBranchRender_CommentsOverrideCommand(t *testing.T) {
	repoRoot := t.TempDir()
	writeBranchRegistryFixture(t, repoRoot, branchRegistryFixtureWithOverrideComment())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdBranch([]string{"render", "-repo-root", repoRoot, "-channel", "main"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdBranch(render main with comments) error: %v\nstderr=%s", err, stderr.String())
	}
	if code != 0 {
		t.Fatalf("cmdBranch(render main with comments) code: got %d want 0\nstderr=%s", code, stderr.String())
	}

	content, err := os.ReadFile(filepath.Join(repoRoot, branchDevcontainerPath))
	if err != nil {
		t.Fatalf("ReadFile(rendered devcontainer with comments): %v", err)
	}
	renderedText := string(content)
	commentBlock := "  // Intent: Preserve decomk's producer/consumer split while moving the GUI stack into explicit, standard system locations and avoiding legacy popup/autostart reminder behavior.\n  // Source: DI-004-20260430-182956\n  \"overrideCommand\": false,"
	if !strings.Contains(renderedText, commentBlock) {
		t.Fatalf("rendered devcontainer comments: missing block %q in\n%s", commentBlock, renderedText)
	}
}

func TestCmdBranchRender_CheckFailsWhenCommentsDrift(t *testing.T) {
	repoRoot := t.TempDir()
	writeBranchRegistryFixture(t, repoRoot, branchRegistryFixtureWithOverrideComment())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdBranch([]string{"render", "-repo-root", repoRoot, "-channel", "main"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdBranch(render main with comments) error: %v\nstderr=%s", err, stderr.String())
	}
	if code != 0 {
		t.Fatalf("cmdBranch(render main with comments) code: got %d want 0\nstderr=%s", code, stderr.String())
	}

	writeBranchRegistryFixture(t, repoRoot, strings.ReplaceAll(
		branchRegistryFixtureWithOverrideComment(),
		"legacy popup/autostart reminder behavior.",
		"legacy popup/autostart reminder behavior changed.",
	))

	stdout.Reset()
	stderr.Reset()
	code, err = cmdBranch([]string{"render", "-repo-root", repoRoot, "-channel", "main", "-check"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdBranch(render -check comment drift) error: got nil want stale error")
	}
	if code != 1 {
		t.Fatalf("cmdBranch(render -check comment drift) code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "is stale") {
		t.Fatalf("cmdBranch(render -check comment drift) error: got %q want stale message", err.Error())
	}
}

func TestCmdBranchRender_RejectsCommentKeyForMissingRenderedField(t *testing.T) {
	repoRoot := t.TempDir()
	writeBranchRegistryFixture(t, repoRoot, branchRegistryFixtureWithImageComment())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := cmdBranch([]string{"render", "-repo-root", repoRoot, "-channel", "main"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("cmdBranch(render main with image comment) error: got nil want comment-key rejection")
	}
	if code != 1 {
		t.Fatalf("cmdBranch(render main with image comment) code: got %d want 1", code)
	}
	if !strings.Contains(err.Error(), "comment key \"image\" does not match a rendered field for channel \"main\"") {
		t.Fatalf("cmdBranch(render main with image comment) error: got %q want comment-key rejection", err.Error())
	}
}

func branchRegistryFixture() string {
	return `{
  "version": 1,
  "devcontainer": {
    "path": ".devcontainer/devcontainer.json",
    "name": "decomk-conf-cswg",
    "overrideCommand": false,
    "containerEnv": {
      "DECOMK_FAIL_NOBOOT": "false"
    },
    "updateRemoteUserUID": false,
    "updateContentCommand": "bash .devcontainer/decomk-stage0.sh updateContent",
    "postCreateCommand": "bash .devcontainer/decomk-stage0.sh postCreate"
  },
  "channels": {
    "main": {
      "build": {
        "dockerfile": "Dockerfile",
        "context": ".."
      },
      "containerEnv": {
        "DECOMK_TOOL_URI": "go:github.com/stevegt/decomk/cmd/decomk@main",
        "DECOMK_CONF_URI": "git:https://github.com/ciwg/decomk-conf-cswg.git?ref=main"
      }
    },
    "testing": {
      "image": "ghcr.io/ciwg/decomk-conf-cswg:testing",
      "containerEnv": {
        "DECOMK_TOOL_URI": "go:github.com/stevegt/decomk/cmd/decomk@testing",
        "DECOMK_CONF_URI": "git:https://github.com/ciwg/decomk-conf-cswg.git?ref=testing"
      }
    },
    "stable": {
      "image": "ghcr.io/ciwg/decomk-conf-cswg:stable",
      "containerEnv": {
        "DECOMK_TOOL_URI": "go:github.com/stevegt/decomk/cmd/decomk@stable",
        "DECOMK_CONF_URI": "git:https://github.com/ciwg/decomk-conf-cswg.git?ref=stable"
      }
    }
  }
}`
}

func branchRegistryFixtureWithOverrideComment() string {
	return strings.Replace(
		branchRegistryFixture(),
		`    "name": "decomk-conf-cswg",`+"\n",
		`    "name": "decomk-conf-cswg",`+"\n"+
			`    "comments": {`+"\n"+
			`      "overrideCommand": [`+"\n"+
			`        "Intent: Preserve decomk's producer/consumer split while moving the GUI stack into explicit, standard system locations and avoiding legacy popup/autostart reminder behavior.",`+"\n"+
			`        "Source: DI-004-20260430-182956"`+"\n"+
			`      ]`+"\n"+
			`    },`+"\n",
		1,
	)
}

func branchRegistryFixtureWithImageComment() string {
	return strings.Replace(
		branchRegistryFixture(),
		`    "name": "decomk-conf-cswg",`+"\n",
		`    "name": "decomk-conf-cswg",`+"\n"+
			`    "comments": {`+"\n"+
			`      "image": [`+"\n"+
			`        "Intent: Image comments should fail on build channels.",`+"\n"+
			`        "Source: DI-018-test"`+"\n"+
			`      ]`+"\n"+
			`    },`+"\n",
		1,
	)
}

func writeBranchRegistryFixture(t *testing.T, repoRoot, content string) {
	t.Helper()
	writeFileFixture(t, filepath.Join(repoRoot, branchRegistryRelPath), []byte(content), 0o644)
}

func writeFileFixture(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func readRenderedDevcontainerFixture(t *testing.T, repoRoot string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repoRoot, branchDevcontainerPath))
	if err != nil {
		t.Fatalf("ReadFile(rendered devcontainer): %v", err)
	}
	withoutComments, err := stripJSONCLineCommentsForInit(content)
	if err != nil {
		t.Fatalf("stripJSONCLineCommentsForInit(rendered devcontainer): %v", err)
	}
	var rendered map[string]any
	if err := json.Unmarshal(withoutComments, &rendered); err != nil {
		t.Fatalf("Unmarshal(rendered devcontainer): %v\n%s", err, string(withoutComments))
	}
	return rendered
}
