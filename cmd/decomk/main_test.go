package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stevegt/decomk/state"
)

func TestLoadDefs_Precedence_ConfigRepoThenExplicit(t *testing.T) {
	t.Parallel()

	// This test encodes the intended config layering:
	//   config repo (lowest) < explicit -config/DECOMK_CONFIG (highest).
	//
	// Each layer may override any key by redefining it.
	home := t.TempDir()

	configRepoConfig := filepath.Join(home, "conf", "decomk.conf")
	if err := os.MkdirAll(filepath.Dir(configRepoConfig), 0o755); err != nil {
		t.Fatalf("MkdirAll(config repo): %v", err)
	}
	if err := os.WriteFile(configRepoConfig, []byte("A: configA\nB: configB\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config repo decomk.conf): %v", err)
	}

	explicit := filepath.Join(t.TempDir(), "decomk.conf")
	if err := os.WriteFile(explicit, []byte("B: explicitB\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(explicit decomk.conf): %v", err)
	}

	defs, paths, err := loadDefs(home, explicit)
	if err != nil {
		t.Fatalf("loadDefs() error: %v", err)
	}

	// Precedence is "last wins": config repo < explicit.
	if got, want := defs["A"], []string{"configA"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("A tokens: got %#v want %#v", got, want)
	}
	if got, want := defs["B"], []string{"explicitB"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("B tokens: got %#v want %#v", got, want)
	}

	if got, want := paths, []string{configRepoConfig, explicit}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths: got %#v want %#v", got, want)
	}
}

func TestSelectTargets(t *testing.T) {
	t.Parallel()

	// This test encodes the isconf-style "action args" behavior:
	//   - positional args select actions/targets,
	//   - action variable args expand via tuple values,
	//   - otherwise args are treated as literal targets,
	//   - and when args are present they override config-derived target tokens.
	cases := []struct {
		name          string
		configTargets []string
		tuples        []string
		actionArgs    []string
		wantTargets   []string
		wantSource    string
	}{
		{
			name:          "action args expand INSTALL",
			configTargets: []string{"configTarget"},
			tuples:        []string{"INSTALL=install-neovim install-codex"},
			actionArgs:    []string{"INSTALL"},
			wantTargets:   []string{"install-neovim", "install-codex"},
			wantSource:    "actionArgs",
		},
		{
			name:        "action args unknown treated as literal target",
			actionArgs:  []string{"install-neovim"},
			wantTargets: []string{"install-neovim"},
			wantSource:  "actionArgs",
		},
		{
			name:        "action args mix expanded and literal",
			tuples:      []string{"INSTALL=one two"},
			actionArgs:  []string{"INSTALL", "extra"},
			wantTargets: []string{"one", "two", "extra"},
			wantSource:  "actionArgs",
		},
		{
			name:          "no args uses config-derived targets",
			configTargets: []string{"Block00_base", "Block10_common"},
			tuples:        []string{"INSTALL=ignored"},
			wantTargets:   []string{"Block00_base", "Block10_common"},
			wantSource:    "configTargets",
		},
		{
			name:        "no args falls back to INSTALL when no config targets",
			tuples:      []string{"INSTALL=one two"},
			wantTargets: []string{"one", "two"},
			wantSource:  "defaultINSTALL",
		},
		{
			name:        "INSTALL last wins",
			tuples:      []string{"INSTALL=old", "INSTALL=new newer"},
			wantTargets: []string{"new", "newer"},
			wantSource:  "defaultINSTALL",
		},
		{
			name:       "no targets means make default goal",
			wantSource: "makeDefaultGoal",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotTargets, gotSource := selectTargets(tc.configTargets, tc.tuples, tc.actionArgs)
			if gotSource != tc.wantSource {
				t.Fatalf("source: got %q want %q", gotSource, tc.wantSource)
			}
			if !reflect.DeepEqual(gotTargets, tc.wantTargets) {
				t.Fatalf("targets: got %#v want %#v", gotTargets, tc.wantTargets)
			}
		})
	}
}

func TestResolveWorkspacesDir_Precedence(t *testing.T) {
	t.Parallel()

	const envKey = "DECOMK_WORKSPACES_DIR"
	old, had := os.LookupEnv(envKey)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(envKey, old)
			return
		}
		_ = os.Unsetenv(envKey)
	})

	// Flag overrides env.
	if err := os.Setenv(envKey, "/from-env"); err != nil {
		t.Fatalf("Setenv: %v", err)
	}
	if got, want := resolveWorkspacesDir("/from-flag"), "/from-flag"; got != want {
		t.Fatalf("flag wins: got %q want %q", got, want)
	}

	// Env overrides default.
	if got, want := resolveWorkspacesDir(""), "/from-env"; got != want {
		t.Fatalf("env wins: got %q want %q", got, want)
	}

	// Default when neither set.
	if err := os.Unsetenv(envKey); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	if got, want := resolveWorkspacesDir(""), defaultWorkspacesDir; got != want {
		t.Fatalf("default: got %q want %q", got, want)
	}
}

func TestRewriteCFlag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		args        []string
		absStartDir string
		want        []string
	}{
		{
			name:        "no -C unchanged",
			args:        []string{"decomk", "plan"},
			absStartDir: "/abs",
			want:        []string{"decomk", "plan"},
		},
		{
			name:        "-C arg rewritten",
			args:        []string{"decomk", "plan", "-C", ".."},
			absStartDir: "/abs",
			want:        []string{"decomk", "plan", "-C", "/abs"},
		},
		{
			name:        "-C= arg rewritten",
			args:        []string{"decomk", "plan", "-C=.."},
			absStartDir: "/abs",
			want:        []string{"decomk", "plan", "-C=/abs"},
		},
		{
			name:        "multiple -C occurrences all rewritten",
			args:        []string{"decomk", "plan", "-C", "..", "-C=.", "-C", "x"},
			absStartDir: "/abs",
			want:        []string{"decomk", "plan", "-C", "/abs", "-C=/abs", "-C", "/abs"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			orig := append([]string(nil), tc.args...)
			got := rewriteCFlag(tc.args, tc.absStartDir)

			if !reflect.DeepEqual(tc.args, orig) {
				t.Fatalf("rewriteCFlag mutated input: got %#v want %#v", tc.args, orig)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rewriteCFlag: got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestResolveLogRoot(t *testing.T) {
	// Note: this test intentionally does not run in parallel because it mutates
	// environment variables (process-global state).

	t.Run("default uses state.DefaultLogDir", func(t *testing.T) {
		t.Setenv("DECOMK_LOG_DIR", "")
		got, explicit, err := resolveLogRoot("")
		if err != nil {
			t.Fatalf("resolveLogRoot() error: %v", err)
		}
		if got != state.DefaultLogDir {
			t.Fatalf("log root: got %q want %q", got, state.DefaultLogDir)
		}
		if explicit {
			t.Fatalf("explicit: got true want false")
		}
	})

	t.Run("env overrides default", func(t *testing.T) {
		envDir := filepath.Join(t.TempDir(), "logs")
		t.Setenv("DECOMK_LOG_DIR", envDir)
		got, explicit, err := resolveLogRoot("")
		if err != nil {
			t.Fatalf("resolveLogRoot() error: %v", err)
		}
		if got != envDir {
			t.Fatalf("log root: got %q want %q", got, envDir)
		}
		if !explicit {
			t.Fatalf("explicit: got false want true")
		}
	})

	t.Run("flag overrides env", func(t *testing.T) {
		envDir := filepath.Join(t.TempDir(), "env-logs")
		flagDir := filepath.Join(t.TempDir(), "flag-logs")
		t.Setenv("DECOMK_LOG_DIR", envDir)
		got, explicit, err := resolveLogRoot(flagDir)
		if err != nil {
			t.Fatalf("resolveLogRoot() error: %v", err)
		}
		if got != flagDir {
			t.Fatalf("log root: got %q want %q", got, flagDir)
		}
		if !explicit {
			t.Fatalf("explicit: got false want true")
		}
	})

	t.Run("flag must be absolute", func(t *testing.T) {
		t.Setenv("DECOMK_LOG_DIR", "")
		_, _, err := resolveLogRoot("relative/path")
		if err == nil {
			t.Fatalf("resolveLogRoot() error: got nil want error")
		}
		if !strings.Contains(err.Error(), "flag -log-dir") {
			t.Fatalf("error: got %q want mention of flag -log-dir", err.Error())
		}
	})

	t.Run("env must be absolute", func(t *testing.T) {
		t.Setenv("DECOMK_LOG_DIR", "relative/path")
		_, _, err := resolveLogRoot("")
		if err == nil {
			t.Fatalf("resolveLogRoot() error: got nil want error")
		}
		if !strings.Contains(err.Error(), "DECOMK_LOG_DIR") {
			t.Fatalf("error: got %q want mention of DECOMK_LOG_DIR", err.Error())
		}
	})
}

func TestCreateRunLogDir_FallbackToHomeLogDir(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	runID := "20000102T030405Z-1234"

	// Use a file (not a directory) as the log root so directory creation fails in
	// a deterministic way, regardless of platform.
	badRoot := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(badRoot, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(badRoot): %v", err)
	}

	plan := &resolvedPlan{
		Home:            home,
		LogRoot:         badRoot,
		LogRootExplicit: false, // default path behavior: allow fallback
	}

	var stderr bytes.Buffer
	dir, err := createRunLogDir(plan, runID, &stderr)
	if err != nil {
		t.Fatalf("createRunLogDir() error: %v", err)
	}

	wantRoot := state.LogDir(home)
	wantDir := filepath.Join(wantRoot, runID)
	if dir != wantDir {
		t.Fatalf("dir: got %q want %q", dir, wantDir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("Stat(dir): %v", err)
	}
	if got := stderr.String(); !strings.Contains(got, "falling back") || !strings.Contains(got, wantRoot) {
		t.Fatalf("stderr: got %q want fallback message mentioning %q", got, wantRoot)
	}
}

func TestCreateRunLogDir_ExplicitDoesNotFallback(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	runID := "20000102T030405Z-1234"

	badRoot := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(badRoot, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(badRoot): %v", err)
	}

	plan := &resolvedPlan{
		Home:            home,
		LogRoot:         badRoot,
		LogRootExplicit: true, // explicit means strict: do not fall back
	}

	var stderr bytes.Buffer
	_, err := createRunLogDir(plan, runID, &stderr)
	if err == nil {
		t.Fatalf("createRunLogDir() error: got nil want error")
	}

	// Explicit log roots should not implicitly create <home>/log.
	if _, err := os.Stat(state.LogDir(home)); !os.IsNotExist(err) {
		t.Fatalf("fallback log dir exists unexpectedly: err=%v", err)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr: got %q want empty", got)
	}
}
