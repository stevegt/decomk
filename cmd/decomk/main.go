// Command decomk is an isconf-inspired bootstrap wrapper for devcontainers.
//
// MVP responsibilities:
//   - Load decomk.conf (and optional decomk.d/*.conf).
//   - Expand macros into make targets + VAR=value tuples.
//   - Write an auditable env snapshot.
//   - Optionally execute GNU make in a persistent stamp directory.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stevegt/decomk/contexts"
	"github.com/stevegt/decomk/expand"
	"github.com/stevegt/decomk/makeexec"
	"github.com/stevegt/decomk/resolve"
	"github.com/stevegt/decomk/state"
)

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

// run is the CLI entrypoint. It returns an exit code (like main) rather than
// calling os.Exit directly, which makes it easy to test in the future.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, usage())
		return 2
	}

	switch args[1] {
	case "-h", "-help", "--help", "help":
		fmt.Fprintln(stdout, usage())
		return 0
	case "plan":
		code, err := cmdPlan(args[2:], stdout, stderr)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return code
		}
		return code
	case "run":
		code, err := cmdRun(args[2:], stdout, stderr)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return code
		}
		return code
	default:
		fmt.Fprintln(stderr, "unknown command:", args[1])
		fmt.Fprintln(stderr, usage())
		return 2
	}
}

// usage returns the top-level help text.
func usage() string {
	return `decomk - devcontainer bootstrap wrapper around make

Usage:
  decomk <command> [flags]

Commands (MVP):
  plan    Print resolved tuples/targets; write env snapshot; do not run make
  run     Resolve, write env snapshot, and run make in the stamp dir
`
}

// commonFlags are the shared flags for subcommands that resolve a context.
type commonFlags struct {
	home        string
	workspace   string
	context     string
	config      string
	makefile    string
	verbose     bool
	maxExpDepth int
}

// addCommonFlags defines flags shared by plan/run.
func addCommonFlags(fs *flag.FlagSet, f *commonFlags) {
	fs.StringVar(&f.home, "home", "", "decomk home directory (overrides DECOMK_HOME)")
	fs.StringVar(&f.workspace, "C", ".", "workspace directory (like make -C)")
	fs.StringVar(&f.context, "context", "", "context key override (also DECOMK_CONTEXT)")
	fs.StringVar(&f.config, "config", "", "config file path override (also DECOMK_CONFIG)")
	fs.StringVar(&f.makefile, "makefile", "", "makefile path override")
	// Note: -v is reserved for future improvements (more logging and plan details).
	fs.BoolVar(&f.verbose, "v", false, "verbose output")
	fs.IntVar(&f.maxExpDepth, "max-expand-depth", 0, "macro expansion depth limit (default 64)")
}

type resolvedPlan struct {
	// Home is the decomk state root (DECOMK_HOME, or /var/decomk by default).
	Home          string
	WorkspaceRoot string
	WorkspaceKey  string
	ContextKey    string

	// ConfigPaths are the config sources that were loaded (in precedence order).
	ConfigPaths []string

	// StampDir is the per-workspace/per-context make working directory.
	StampDir string
	// EnvFile is the resolved env snapshot written for auditing/debugging.
	EnvFile  string
	Makefile string

	// Expanded is the flattened macro expansion result before partitioning.
	Expanded []string
	// Tuples are the NAME=value entries passed on make's argv.
	Tuples []string
	// Targets are the make targets passed on make's argv.
	Targets []string
}

// cmdPlan resolves the context and writes an env snapshot, but does not invoke make.
//
// This is intended to be safe to run in lifecycle hooks where you want to see
// what decomk *would* do, without making changes.
func cmdPlan(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("decomk plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f commonFlags
	addCommonFlags(fs, &f)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}

	plan, err := resolvePlanFromFlags(f)
	if err != nil {
		return 1, err
	}

	if err := writeEnvSnapshot(plan.EnvFile, plan); err != nil {
		return 1, err
	}

	fmt.Fprintf(stdout, "home: %s\n", plan.Home)
	fmt.Fprintf(stdout, "workspaceRoot: %s\n", plan.WorkspaceRoot)
	fmt.Fprintf(stdout, "workspaceKey: %s\n", plan.WorkspaceKey)
	fmt.Fprintf(stdout, "contextKey: %s\n", plan.ContextKey)
	fmt.Fprintf(stdout, "config: %s\n", strings.Join(plan.ConfigPaths, ", "))
	fmt.Fprintf(stdout, "env: %s\n", plan.EnvFile)
	fmt.Fprintf(stdout, "stampDir: %s\n", plan.StampDir)
	if plan.Makefile != "" {
		fmt.Fprintf(stdout, "makefile: %s\n", plan.Makefile)
	}
	fmt.Fprintln(stdout)

	fmt.Fprintln(stdout, "tuples:")
	for _, t := range plan.Tuples {
		fmt.Fprintf(stdout, "  %s\n", t)
	}
	fmt.Fprintln(stdout, "targets:")
	for _, t := range plan.Targets {
		fmt.Fprintf(stdout, "  %s\n", t)
	}
	return 0, nil
}

// cmdRun resolves the context, writes an env snapshot, and invokes make in a
// persistent stamp directory.
//
// The stamp directory is outside the workspace repo so that re-running decomk
// doesn't dirty the repo with generated state.
func cmdRun(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("decomk run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f commonFlags
	addCommonFlags(fs, &f)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}

	plan, err := resolvePlanFromFlags(f)
	if err != nil {
		return 1, err
	}

	if plan.Makefile == "" {
		return 1, fmt.Errorf("no Makefile found; use -makefile to set an explicit path")
	}

	if err := writeEnvSnapshot(plan.EnvFile, plan); err != nil {
		return 1, err
	}

	// Prevent concurrent stamp mutation for this workspace.
	lock, err := state.LockFile(state.WorkspaceLockPath(plan.Home, plan.WorkspaceKey))
	if err != nil {
		return 1, fmt.Errorf("lock workspace: %w", err)
	}
	defer lock.Close()

	if err := state.EnsureDir(plan.StampDir); err != nil {
		return 1, err
	}
	if err := state.TouchExistingStamps(plan.StampDir, time.Now()); err != nil {
		return 1, fmt.Errorf("touch stamps: %w", err)
	}

	// Include sub-second resolution and pid to avoid collisions when two runs start
	// close together (otherwise one run can clobber the other's audit log).
	runID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + strconv.Itoa(os.Getpid())
	auditDir, err := createAuditDir(plan.Home, plan.WorkspaceKey, runID)
	if err != nil {
		return 1, err
	}
	logPath := filepath.Join(auditDir, "make.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return 1, err
	}
	defer logFile.Close()

	teeOut := io.MultiWriter(stdout, logFile)
	teeErr := io.MultiWriter(stderr, logFile)

	tuples := append([]string(nil), plan.Tuples...)
	tuples = appendIfMissingTuple(tuples, "DECOMK_WORKSPACE_ROOT", plan.WorkspaceRoot)
	tuples = appendIfMissingTuple(tuples, "DECOMK_STAMPDIR", plan.StampDir)
	tuples = appendIfMissingTuple(tuples, "DECOMK_CONTEXT", plan.ContextKey)

	env := withEnv(os.Environ(), map[string]string{
		"DECOMK_HOME":           plan.Home,
		"DECOMK_WORKSPACE_ROOT": plan.WorkspaceRoot,
		"DECOMK_STAMPDIR":       plan.StampDir,
		"DECOMK_CONTEXT":        plan.ContextKey,
	})

	exitCode, runErr := makeexec.Run(plan.StampDir, plan.Makefile, tuples, plan.Targets, env, teeOut, teeErr)
	if runErr != nil {
		// Preserve make's exit status.
		return exitCode, fmt.Errorf("make failed (exit %d); log: %s: %w", exitCode, logPath, runErr)
	}
	return 0, nil
}

// createAuditDir creates a per-run audit directory and returns its path.
//
// The directory must be unique for each invocation so audit logs never
// overwrite each other.
func createAuditDir(home, workspaceKey, runID string) (string, error) {
	base := state.AuditDir(home, workspaceKey, runID)
	if err := state.EnsureDir(filepath.Dir(base)); err != nil {
		return "", err
	}

	dir := base
	for i := 0; ; i++ {
		if err := os.Mkdir(dir, 0o755); err != nil {
			if os.IsExist(err) {
				dir = base + "-" + strconv.Itoa(i+1)
				continue
			}
			return "", err
		}
		return dir, nil
	}
}

// resolvePlanFromFlags builds a fully-resolved plan from the user-facing flags:
// it locates config, resolves context, expands macros, and computes state paths.
func resolvePlanFromFlags(f commonFlags) (*resolvedPlan, error) {
	home, err := state.Home(f.home)
	if err != nil {
		return nil, err
	}

	workspaceRoot, err := state.WorkspaceRoot(f.workspace)
	if err != nil {
		return nil, err
	}

	githubRepo := os.Getenv("GITHUB_REPOSITORY")
	workspaceKey, err := state.WorkspaceKey(workspaceRoot, githubRepo)
	if err != nil {
		return nil, err
	}

	explicitConfig := f.config
	if explicitConfig == "" {
		explicitConfig = os.Getenv("DECOMK_CONFIG")
	}

	defs, configPaths, err := loadDefs(home, workspaceRoot, explicitConfig)
	if err != nil {
		return nil, err
	}

	contextKey, err := selectContextKey(defs, f.context)
	if err != nil {
		return nil, err
	}

	seed := seedTokens(defs, contextKey)
	expanded, err := expand.ExpandTokens(expand.Defs(defs), seed, expand.Options{MaxDepth: f.maxExpDepth})
	if err != nil {
		return nil, err
	}
	tuples, targets := resolve.Partition(expanded)

	contextPathKey := state.SafeComponent(contextKey)
	stampDir := state.StampDir(home, workspaceKey, contextPathKey)
	envFile := state.EnvFile(home, workspaceKey, contextPathKey)

	makefile := f.makefile
	if makefile == "" {
		makefile = findDefaultMakefile(home, workspaceRoot, explicitConfig)
	}
	if makefile != "" && !fileExists(makefile) {
		return nil, fmt.Errorf("makefile not found: %s", makefile)
	}

	return &resolvedPlan{
		Home:          home,
		WorkspaceRoot: workspaceRoot,
		WorkspaceKey:  workspaceKey,
		ContextKey:    contextKey,
		ConfigPaths:   configPaths,
		StampDir:      stampDir,
		EnvFile:       envFile,
		Makefile:      makefile,
		Expanded:      expanded,
		Tuples:        tuples,
		Targets:       targets,
	}, nil
}

// loadDefs loads decomk.conf trees from multiple sources and merges them.
//
// Precedence is "last wins" (higher precedence overrides lower):
//
// Future: extend precedence beyond these three sources (e.g., per-owner/per-org
// defaults, container-image defaults, etc.) while keeping the model auditable.
//
//  1. config repo decomk.conf (lowest; optional)
//  2. repo-local decomk.conf (optional)
//  3. explicit -config / DECOMK_CONFIG (highest; optional)
//
// Each source is loaded via contexts.LoadTree so it can also include a sibling
// decomk.d/*.conf directory.
func loadDefs(home, workspaceRoot, explicitConfig string) (defs contexts.Defs, paths []string, err error) {
	// Precedence: config repo (lowest) -> repo-local -> explicit override (highest).
	var sources []string

	configRepo := filepath.Join(home, "repos", "decomk-config", "decomk.conf")
	if fileExists(configRepo) {
		sources = append(sources, configRepo)
	}

	repoLocal := filepath.Join(workspaceRoot, "decomk.conf")
	if fileExists(repoLocal) {
		sources = append(sources, repoLocal)
	}
	if explicitConfig != "" {
		if !fileExists(explicitConfig) {
			return nil, nil, fmt.Errorf("config file not found: %s", explicitConfig)
		}
		sources = append(sources, explicitConfig)
	}

	if len(sources) == 0 {
		return nil, nil, fmt.Errorf("no config found; tried %s and %s; set -config or DECOMK_CONFIG", configRepo, repoLocal)
	}

	// Load lowest-precedence first.
	defs = make(contexts.Defs)
	for _, p := range sources {
		tree, e := contexts.LoadTree(p)
		if e != nil {
			return nil, nil, e
		}
		defs = contexts.Merge(defs, tree)
	}

	paths = append([]string(nil), sources...)
	return defs, paths, nil
}

// selectContextKey chooses which context key to apply.
//
// Selection order (first match wins):
//  1. -context
//  2. DECOMK_CONTEXT
//  3. GITHUB_REPOSITORY ("owner/repo"), then just "repo"
//  4. DEFAULT
func selectContextKey(defs contexts.Defs, flagContext string) (string, error) {
	if flagContext != "" {
		if _, ok := defs[flagContext]; !ok {
			return "", fmt.Errorf("context not found: %q", flagContext)
		}
		return flagContext, nil
	}
	if env := os.Getenv("DECOMK_CONTEXT"); env != "" {
		if _, ok := defs[env]; !ok {
			return "", fmt.Errorf("context not found: %q (from DECOMK_CONTEXT)", env)
		}
		return env, nil
	}

	var candidates []string
	if gr := os.Getenv("GITHUB_REPOSITORY"); gr != "" {
		candidates = append(candidates, gr)
		if _, repo, ok := strings.Cut(gr, "/"); ok && repo != "" {
			candidates = append(candidates, repo)
		}
	}
	candidates = append(candidates, "DEFAULT")

	for _, c := range candidates {
		if _, ok := defs[c]; ok {
			return c, nil
		}
	}
	return "", fmt.Errorf("no matching context found; tried %v", candidates)
}

// seedTokens returns the initial macro tokens to expand for a context.
//
// The common idiom is "DEFAULT + <context>" composition. To reduce surprising
// duplicates, if a context explicitly includes DEFAULT in its own token list we
// do not add it implicitly here.
func seedTokens(defs contexts.Defs, contextKey string) []string {
	var seed []string
	if contextKey != "DEFAULT" {
		if _, ok := defs["DEFAULT"]; ok {
			// Users sometimes include "DEFAULT" explicitly inside a context
			// definition (isconf style). If so, don't also seed it implicitly.
			if ctx, ok := defs[contextKey]; !ok || !containsToken(ctx, "DEFAULT") {
				seed = append(seed, "DEFAULT")
			}
		}
	}
	seed = append(seed, contextKey)
	return seed
}

// containsToken reports whether tokens contains want.
func containsToken(tokens []string, want string) bool {
	for _, t := range tokens {
		if t == want {
			return true
		}
	}
	return false
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// appendIfMissingTuple appends a NAME=value tuple unless a tuple with NAME is
// already present (in which case the existing value wins).
func appendIfMissingTuple(tuples []string, name, value string) []string {
	for _, t := range tuples {
		k, _, ok := resolve.SplitTuple(t)
		if ok && k == name {
			return tuples
		}
	}
	return append(tuples, name+"="+value)
}

// withEnv returns base plus additional KEY=VALUE assignments.
//
// Any keys present in set replace existing entries in base (by filtering them
// out first). New entries are appended in sorted-key order for stable logs.
func withEnv(base []string, set map[string]string) []string {
	// Preserve ordering for readability/debugging, but ensure the last assignment
	// wins by filtering existing keys.
	keep := make([]string, 0, len(base))
	for _, kv := range base {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, shouldSet := set[k]; shouldSet {
			continue
		}
		keep = append(keep, kv)
	}

	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		keep = append(keep, k+"="+set[k])
	}
	return keep
}

// findDefaultMakefile picks a default Makefile path when -makefile is not set.
//
// decomk's long-term model is that the Makefile is part of the shared "config
// repo" (under DECOMK_HOME). Repo-local decomk.conf is intended as an overlay,
// not as the canonical source of make recipes.
//
// Selection order (first match wins):
//  1. <DECOMK_HOME>/repos/decomk-config/Makefile
//  2. sibling of explicitConfig (if non-empty)
//  3. workspaceRoot/Makefile
func findDefaultMakefile(home, workspaceRoot, explicitConfig string) string {
	configRepoMakefile := filepath.Join(home, "repos", "decomk-config", "Makefile")
	if fileExists(configRepoMakefile) {
		return configRepoMakefile
	}
	if explicitConfig != "" {
		candidate := filepath.Join(filepath.Dir(explicitConfig), "Makefile")
		if fileExists(candidate) {
			return candidate
		}
	}
	repoLocalMakefile := filepath.Join(workspaceRoot, "Makefile")
	if fileExists(repoLocalMakefile) {
		return repoLocalMakefile
	}
	return ""
}

// writeEnvSnapshot writes a shell-friendly env file that captures the resolved
// tuples for the run.
//
// This file is intentionally simple: it is designed to be sourced by scripts
// and nested make invocations without requiring eval.
func writeEnvSnapshot(path string, plan *resolvedPlan) error {
	if err := state.EnsureParentDir(path); err != nil {
		return err
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "# generated by decomk; do not edit\n")
	fmt.Fprintf(f, "# time: %s\n", now)
	fmt.Fprintf(f, "# workspaceRoot: %s\n", plan.WorkspaceRoot)
	fmt.Fprintf(f, "# contextKey: %s\n", plan.ContextKey)
	fmt.Fprintf(f, "# config: %s\n", strings.Join(plan.ConfigPaths, ", "))
	fmt.Fprintln(f)

	// Export computed helpers for recipes/scripts.
	writeExport(f, "DECOMK_HOME", plan.Home)
	writeExport(f, "DECOMK_WORKSPACE_ROOT", plan.WorkspaceRoot)
	writeExport(f, "DECOMK_CONTEXT", plan.ContextKey)
	writeExport(f, "DECOMK_STAMPDIR", plan.StampDir)

	// Export config-provided tuples.
	for _, t := range plan.Tuples {
		k, v, ok := resolve.SplitTuple(t)
		if !ok {
			continue
		}
		writeExport(f, k, v)
	}

	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeExport emits one export line using conservative shell quoting.
func writeExport(w io.Writer, name, value string) {
	fmt.Fprintf(w, "export %s=%s\n", name, shellQuote(value))
}

// shellQuote produces a POSIX-shell-safe single-quoted string.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// Close/open around any embedded single quote.
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
