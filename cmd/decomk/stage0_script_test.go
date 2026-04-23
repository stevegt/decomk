package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stevegt/decomk/stage0"
)

func TestStage0ScriptFailNoBootPolicy(t *testing.T) {
	scriptPath, baseEnv := writeStage0ScriptFixture(t)

	t.Run("continue mode writes artifacts and exits zero", func(t *testing.T) {
		env := cloneEnvMap(baseEnv)
		env["DECOMK_FAIL_NOBOOT"] = "false"
		env["FAKE_DECOMK_RC"] = "13"

		exitCode, output := runStage0Script(t, scriptPath, env)
		if exitCode != 0 {
			t.Fatalf("exit code: got %d want 0\noutput:\n%s", exitCode, output)
		}
		if !strings.Contains(output, "continuing boot because DECOMK_FAIL_NOBOOT=false") {
			t.Fatalf("output missing continue-mode warning:\n%s", output)
		}
		if !strings.Contains(output, "SELFTEST PASS stage0-id phase=postCreate") {
			t.Fatalf("output missing stage0 identity marker:\n%s", output)
		}
		if !strings.Contains(output, "wrote failure marker: "+baseEnv["STAGE0_MARKER_PATH"]) {
			t.Fatalf("output missing failure-marker write line:\n%s", output)
		}
		if !strings.Contains(output, "wrote failure log: "+baseEnv["STAGE0_LOG_PATH"]) {
			t.Fatalf("output missing failure-log write line:\n%s", output)
		}
		if !strings.Contains(output, "wrote fallback hint: "+baseEnv["STAGE0_MOTD_FALLBACK"]) &&
			!strings.Contains(output, "wrote MOTD hint: /etc/motd.d/80-decomk-stage0") &&
			!strings.Contains(output, "updated MOTD hint: /etc/motd.d/80-decomk-stage0") {
			t.Fatalf("output missing MOTD/fallback hint write line:\n%s", output)
		}

		markerContents, err := os.ReadFile(baseEnv["STAGE0_MARKER_PATH"])
		if err != nil {
			t.Fatalf("ReadFile(marker): %v", err)
		}
		if !strings.Contains(string(markerContents), "decomk_fail_noboot=false") {
			t.Fatalf("marker missing fail policy value:\n%s", string(markerContents))
		}
	})

	t.Run("fail mode exits non-zero and keeps artifacts", func(t *testing.T) {
		env := cloneEnvMap(baseEnv)
		env["DECOMK_FAIL_NOBOOT"] = "true"
		env["FAKE_DECOMK_RC"] = "17"

		exitCode, output := runStage0Script(t, scriptPath, env)
		if exitCode != 17 {
			t.Fatalf("exit code: got %d want 17\noutput:\n%s", exitCode, output)
		}
		if !strings.Contains(output, "DECOMK_FAIL_NOBOOT=true; exiting non-zero (rc=17)") {
			t.Fatalf("output missing fail-mode line:\n%s", output)
		}
		if !strings.Contains(output, "SELFTEST PASS stage0-id phase=postCreate") {
			t.Fatalf("output missing stage0 identity marker:\n%s", output)
		}

		markerContents, err := os.ReadFile(baseEnv["STAGE0_MARKER_PATH"])
		if err != nil {
			t.Fatalf("ReadFile(marker): %v", err)
		}
		if !strings.Contains(string(markerContents), "decomk_fail_noboot=true") {
			t.Fatalf("marker missing fail policy value:\n%s", string(markerContents))
		}
	})

	t.Run("invalid value fails explicitly", func(t *testing.T) {
		env := cloneEnvMap(baseEnv)
		env["DECOMK_FAIL_NOBOOT"] = "maybe"
		env["FAKE_DECOMK_RC"] = "0"

		exitCode, output := runStage0Script(t, scriptPath, env)
		if exitCode == 0 {
			t.Fatalf("exit code: got 0 want non-zero\noutput:\n%s", output)
		}
		if !strings.Contains(output, "invalid DECOMK_FAIL_NOBOOT=maybe") {
			t.Fatalf("output missing invalid-policy message:\n%s", output)
		}
	})
}

func writeStage0ScriptFixture(t *testing.T) (string, map[string]string) {
	t.Helper()

	rendered, err := stage0.RenderStage0Script(initStage0ScriptTemplate)
	if err != nil {
		t.Fatalf("RenderStage0Script(): %v", err)
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	logDir := filepath.Join(root, "log")
	binDir := filepath.Join(root, "bin")
	gobin := filepath.Join(root, "gobin")
	confDir := filepath.Join(home, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(confDir): %v", err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(binDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "decomk.conf"), []byte("DEFAULT: TEST_ACTION='echo ok'\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(decomk.conf): %v", err)
	}

	fakeGoPath := filepath.Join(binDir, "go")
	if err := os.WriteFile(fakeGoPath, []byte(`#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
if [[ "$cmd" == "env" ]]; then
  case "${2:-}" in
    GOBIN)
      printf '%s\n' "${GOBIN:-}"
      ;;
    GOPATH)
      printf '%s\n' "${GOPATH:-}"
      ;;
    *)
      exit 1
      ;;
  esac
  exit 0
fi
if [[ "$cmd" == "install" ]]; then
  target="${GOBIN:-${GOPATH}/bin}"
  mkdir -p "$target"
  cat >"$target/decomk" <<'EOS'
#!/usr/bin/env bash
set -euo pipefail
rc="${FAKE_DECOMK_RC:-0}"
if [[ "$rc" == "0" ]]; then
  echo "fake decomk success"
  exit 0
fi
echo "fake decomk failure rc=$rc" >&2
exit "$rc"
EOS
  chmod +x "$target/decomk"
  exit 0
fi
echo "unexpected fake go invocation: $*" >&2
exit 1
`), 0o755); err != nil {
		t.Fatalf("WriteFile(fake go): %v", err)
	}

	scriptPath := filepath.Join(root, "decomk-stage0.sh")
	if err := os.WriteFile(scriptPath, rendered, 0o755); err != nil {
		t.Fatalf("WriteFile(stage0 script): %v", err)
	}

	baseEnv := map[string]string{
		"DECOMK_HOME":          home,
		"DECOMK_LOG_DIR":       logDir,
		"DECOMK_TOOL_URI":      "go:example.com/fake/decomk@v0.0.1",
		"DECOMK_CONF_URI":      "",
		"GOBIN":                gobin,
		"GOPATH":               filepath.Join(root, "gopath"),
		"PATH":                 binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"STAGE0_MARKER_PATH":   filepath.Join(home, "stage0", "failure", "latest-postCreate.marker"),
		"STAGE0_LOG_PATH":      filepath.Join(home, "stage0", "failure", "latest-postCreate.log"),
		"STAGE0_MOTD_FALLBACK": filepath.Join(home, "stage0", "failure", "motd.txt"),
	}
	return scriptPath, baseEnv
}

func runStage0Script(t *testing.T, scriptPath string, envMap map[string]string) (int, string) {
	t.Helper()

	cmd := exec.Command(scriptPath, "postCreate", "TEST_ACTION")
	env := os.Environ()
	for key, value := range envMap {
		if strings.HasPrefix(key, "STAGE0_") {
			continue
		}
		env = append(env, key+"="+value)
	}
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(output)
	}
	exitErr := new(exec.ExitError)
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(output)
	}
	t.Fatalf("runStage0Script() unexpected error: %v\noutput:\n%s", err, string(output))
	return 0, ""
}

func cloneEnvMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
