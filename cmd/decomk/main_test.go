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

func TestResolveTuplePassThroughs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		tuples  []string
		env     map[string]string
		want    []string
		wantErr string
	}{
		{
			name:   "uses incoming env value",
			tuples: []string{"BAX=27", "BAX=$", "OTHER=x"},
			env: map[string]string{
				"BAX": "42",
			},
			want: []string{"BAX=27", "BAX=42", "OTHER=x"},
		},
		{
			name:   "uses prior tuple fallback when env is unset",
			tuples: []string{"BAX=27", "BAX=$"},
			env:    map[string]string{},
			want:   []string{"BAX=27", "BAX=27"},
		},
		{
			name:    "fails when env is unset and no fallback tuple exists",
			tuples:  []string{"BAX=$"},
			env:     map[string]string{},
			wantErr: "tuple BAX=$ requires BAX in environment or a prior tuple fallback",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveTuplePassThroughs(tc.tuples, tc.env)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveTuplePassThroughs() error: got nil want %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("resolveTuplePassThroughs() error: got %q want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTuplePassThroughs() error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("resolveTuplePassThroughs() tuples: got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestSelectTargets_PassthroughInstallTuple(t *testing.T) {
	t.Parallel()

	tuples, err := resolveTuplePassThroughs(
		[]string{"INSTALL=$"},
		map[string]string{"INSTALL": "install-neovim install-codex"},
	)
	if err != nil {
		t.Fatalf("resolveTuplePassThroughs(): %v", err)
	}

	gotTargets, gotSource := selectTargets(nil, tuples, nil)
	wantTargets := []string{"install-neovim", "install-codex"}
	if gotSource != "defaultINSTALL" {
		t.Fatalf("source: got %q want %q", gotSource, "defaultINSTALL")
	}
	if !reflect.DeepEqual(gotTargets, wantTargets) {
		t.Fatalf("targets: got %#v want %#v", gotTargets, wantTargets)
	}
}

func TestCanonicalEnvTuplesAndMakeInvocationParity(t *testing.T) {
	t.Parallel()

	plan := &resolvedPlan{
		Home:     "/tmp/decomk-home",
		StampDir: "/tmp/decomk-home/stamps",
		Tuples:   []string{"DECOMK_TOOL_URI=config-tool-uri", "CUSTOM=ok"},
		ContextKeys: []string{
			"DEFAULT",
			"repo1",
		},
		WorkspaceRepos: []workspaceRepo{
			{Name: "repo1"},
		},
	}
	incomingEnv := map[string]string{
		"DECOMK_TOOL_URI": "env-tool-uri",
		"DECOMK_CONF_URI": "env-conf-uri",
		"NOT_INCLUDED":    "ignored",
	}
	targets := []string{"Block00_base", "Block10_common"}

	cookedTuples := canonicalEnvTuples(plan, targets, false, incomingEnv)
	effective := effectiveTupleValues(cookedTuples)

	// Config tuples must override incoming DECOMK_* pass-through values.
	if got, want := effective["DECOMK_TOOL_URI"], "config-tool-uri"; got != want {
		t.Fatalf("DECOMK_TOOL_URI: got %q want %q", got, want)
	}
	// Incoming DECOMK_* values without tuple overrides should still pass through.
	if got, want := effective["DECOMK_CONF_URI"], "env-conf-uri"; got != want {
		t.Fatalf("DECOMK_CONF_URI: got %q want %q", got, want)
	}
	// Computed values must still override any earlier values.
	if got, want := effective["DECOMK_HOME"], plan.Home; got != want {
		t.Fatalf("DECOMK_HOME: got %q want %q", got, want)
	}
	if _, ok := effective["NOT_INCLUDED"]; ok {
		t.Fatalf("NOT_INCLUDED should not be present in canonical env tuples")
	}

	makeTuples, makeEnv := makeInvocation(
		[]string{"PATH=/usr/bin", "DECOMK_CONF_URI=base-uri"},
		cookedTuples,
	)
	if !reflect.DeepEqual(makeTuples, cookedTuples) {
		t.Fatalf("make tuples: got %#v want %#v", makeTuples, cookedTuples)
	}
	makeEnvMap := envMapFromList(makeEnv)
	for name, want := range effective {
		if got := makeEnvMap[name]; got != want {
			t.Fatalf("make env %s: got %q want %q", name, got, want)
		}
	}
}

func TestResolveWorkspacesDir_Precedence(t *testing.T) {
	t.Parallel()

	const envKey = "DECOMK_WORKSPACES_DIR"
	old, had := os.LookupEnv(envKey)
	t.Cleanup(func() {
		if had {
			if cleanupErr := os.Setenv(envKey, old); cleanupErr != nil {
				t.Errorf("cleanup Setenv(%s): %v", envKey, cleanupErr)
			}
			return
		}
		if cleanupErr := os.Unsetenv(envKey); cleanupErr != nil {
			t.Errorf("cleanup Unsetenv(%s): %v", envKey, cleanupErr)
		}
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

func TestCmdRun_StampDirAndIdempotentTarget(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	origArgs := append([]string(nil), os.Args...)
	t.Cleanup(func() {
		if cleanupErr := os.Chdir(origWD); cleanupErr != nil {
			t.Errorf("cleanup Chdir(origWD): %v", cleanupErr)
		}
		os.Args = origArgs
	})

	home := t.TempDir()
	logRoot := filepath.Join(t.TempDir(), "logs")
	workspacesDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "decomk.conf")
	makefilePath := filepath.Join(t.TempDir(), "Makefile")

	if err := os.WriteFile(configPath, []byte("DEFAULT:\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(configPath): %v", err)
	}

	makefile := strings.Join([]string{
		"SHELL := /bin/bash",
		".ONESHELL:",
		".SHELLFLAGS := -euo pipefail -c",
		".RECIPEPREFIX := >",
		"",
		"stamp-idempotent:",
		">count_file=\"$(DECOMK_HOME)/counter\"",
		">count=0",
		">if [[ -f \"$$count_file\" ]]; then count=\"$$(cat \"$$count_file\")\"; fi",
		">count=$$((count + 1))",
		">echo \"$$count\" > \"$$count_file\"",
		">touch \"$@\"",
		"",
	}, "\n")
	if err := os.WriteFile(makefilePath, []byte(makefile), 0o600); err != nil {
		t.Fatalf("WriteFile(makefilePath): %v", err)
	}

	args := []string{
		"-C", origWD,
		"-home", home,
		"-log-dir", logRoot,
		"-workspaces", workspacesDir,
		"-config", configPath,
		"-makefile", makefilePath,
		"-make-as-root=false",
		"stamp-idempotent",
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Intent: Ensure decomk runs make in the stamp directory and preserves
	// idempotent file-target behavior (non-phony targets should not re-run when
	// their stamp file already exists).
	// Source: DI-001-20260309-172358 (TODO/001)
	code, err := cmdRun(args, &stdout, &stderr)
	if err != nil {
		t.Fatalf("first cmdRun() error: %v (stderr=%q)", err, stderr.String())
	}
	if code != 0 {
		t.Fatalf("first cmdRun() code: got %d want 0 (stderr=%q)", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()

	code, err = cmdRun(args, &stdout, &stderr)
	if err != nil {
		t.Fatalf("second cmdRun() error: %v (stderr=%q)", err, stderr.String())
	}
	if code != 0 {
		t.Fatalf("second cmdRun() code: got %d want 0 (stderr=%q)", code, stderr.String())
	}

	counterPath := filepath.Join(home, "counter")
	counterRaw, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("ReadFile(counter): %v", err)
	}
	if got := strings.TrimSpace(string(counterRaw)); got != "1" {
		t.Fatalf("counter: got %q want %q", got, "1")
	}

	stampPath := filepath.Join(state.StampDir(home), "stamp-idempotent")
	if _, err := os.Stat(stampPath); err != nil {
		t.Fatalf("Stat(stampPath): %v", err)
	}

	unexpectedLocalStampPath := filepath.Join(filepath.Dir(makefilePath), "stamp-idempotent")
	if _, err := os.Stat(unexpectedLocalStampPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected local stamp path exists: %s (err=%v)", unexpectedLocalStampPath, err)
	}
}
