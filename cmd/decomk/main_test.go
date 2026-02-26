package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDefs_Precedence_ConfigRepoThenExplicit(t *testing.T) {
	t.Parallel()

	// This test encodes the intended config layering:
	//   config repo (lowest) < explicit -config/DECOMK_CONFIG (highest).
	//
	// Each layer may override any key by redefining it.
	home := t.TempDir()

	configRepoConfig := filepath.Join(home, "conf", "etc", "decomk.conf")
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
