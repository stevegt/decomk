// Command decomk is an isconf-inspired bootstrap wrapper for devcontainers.
//
// MVP responsibilities:
//   - Load decomk.conf (and optional decomk.d/*.conf).
//   - Expand macros into make targets + VAR=value tuples.
//   - Write a shell-friendly env file (env.sh) for other processes to source.
//   - Optionally execute GNU make in a persistent stamp directory.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
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
  decomk <command> [flags] [ARGS...]

Commands (MVP):
  plan    Print resolved tuples/targets + env exports; run make -n (dry-run); do not write env export file
  run     Resolve, write env export file, and run make in the stamp dir

ARGS:
  Positional args are interpreted isconf-style:
    - If an arg matches a resolved tuple variable name (e.g. INSTALL), its value
      is split on whitespace to produce make targets.
    - Otherwise, the arg is treated as a literal make target name.
`
}

// commonFlags are the shared flags for subcommands that resolve a context.
type commonFlags struct {
	home          string
	logDir        string
	startDir      string
	workspacesDir string
	context       string
	config        string
	toolRepo      string
	confRepo      string
	makefile      string
	verbose       bool
	maxExpDepth   int
}

// addCommonFlags defines flags shared by plan/run.
func addCommonFlags(fs *flag.FlagSet, f *commonFlags) {
	fs.StringVar(&f.home, "home", "", "decomk home directory (overrides DECOMK_HOME)")
	fs.StringVar(&f.logDir, "log-dir", "", "per-run log root directory (absolute path; overrides DECOMK_LOG_DIR; default /var/log/decomk)")
	fs.StringVar(&f.startDir, "C", ".", "starting directory (like make -C)")
	fs.StringVar(&f.workspacesDir, "workspaces", "", "workspaces root directory to scan (overrides DECOMK_WORKSPACES_DIR; default /workspaces)")
	fs.StringVar(&f.context, "context", "", "context key override (also DECOMK_CONTEXT)")
	fs.StringVar(&f.config, "config", "", "config file path override (also DECOMK_CONFIG)")
	fs.StringVar(&f.toolRepo, "tool-repo", "", "decomk tool repo URL to clone/pull into <home>/decomk (also DECOMK_TOOL_REPO)")
	fs.StringVar(&f.confRepo, "conf-repo", "", "config repo URL to clone/pull into <home>/conf (also DECOMK_CONF_REPO)")
	fs.StringVar(&f.makefile, "makefile", "", "makefile path override")
	// Note: -v is reserved for future improvements (more logging and plan details).
	fs.BoolVar(&f.verbose, "v", false, "verbose output")
	fs.IntVar(&f.maxExpDepth, "max-expand-depth", 0, "macro expansion depth limit (default 64)")
}

type resolvedPlan struct {
	// Home is the decomk state root (DECOMK_HOME, or /var/decomk by default).
	Home string

	// LogRoot is the preferred root directory for per-run logs.
	//
	// decomk prefers /var/log/decomk so logs can be managed separately from state
	// under DECOMK_HOME. When LogRoot is the default and not writable (common in
	// non-root environments), decomk may fall back to <DECOMK_HOME>/log.
	LogRoot string

	// LogRootExplicit reports whether LogRoot was explicitly set by the user via
	// -log-dir or DECOMK_LOG_DIR.
	//
	// If this is true and decomk cannot create the per-run log directory, decomk
	// returns an error instead of silently falling back to another location.
	LogRootExplicit bool

	// WorkspaceRepos are the directories under the workspaces root that were
	// considered during resolution.
	//
	// The workspace repo list is used only for config selection and does not imply
	// that decomk will read or write any repo-local state.
	WorkspaceRepos []workspaceRepo

	// ContextKeys are the config keys seeded for expansion, in order.
	//
	// In the common case this is DEFAULT plus one key per discovered workspace
	// (when that key exists in the loaded config).
	ContextKeys []string

	// ConfigPaths are the config sources that were loaded (in precedence order).
	ConfigPaths []string

	// StampDir is decomk's global make working directory (the stamps directory).
	//
	// decomk uses a single stamp directory for the whole container because it is
	// intended to configure the container (tools, caches, etc.), not to manage
	// per-repo build artifacts.
	StampDir string
	// EnvFile is the shell-friendly env export file written for other processes to source.
	EnvFile  string
	Makefile string

	// Expanded is the flattened macro expansion result before partitioning.
	Expanded []string
	// Tuples are the NAME=value entries passed on make's argv.
	Tuples []string
	// Targets are the make targets passed on make's argv.
	Targets []string
}

// cmdPlan resolves config and prints what decomk would do, without running real
// make or modifying stamp files.
//
// Specifically:
//   - it prints the env exports that would be written to <DECOMK_HOME>/env.sh
//   - and it invokes make with -n (GNU make dry-run) to show what recipes would run
//
// This is intended to be safe to run in lifecycle hooks where you want to see
// what decomk *would* do, without making changes to stamps or the env export
// file.
//
// Note: plan still performs the normal bootstrap/update steps:
//   - it may self-update the decomk tool repo under <DECOMK_HOME>/decomk
//   - it may clone/pull the config repo under <DECOMK_HOME>/conf (when configured)
//   - it may create <DECOMK_HOME>/stamps if it does not exist (so make -n can run)
func cmdPlan(args []string, stdout, stderr io.Writer) (int, error) {
	return cmdExecute(args, stdout, stderr, execModePlan)
}

// cmdRun resolves the context, writes an env export file, and invokes make in a
// persistent stamp directory.
//
// The stamp directory is outside the workspace repo so that re-running decomk
// doesn't dirty the repo with generated state.
func cmdRun(args []string, stdout, stderr io.Writer) (int, error) {
	return cmdExecute(args, stdout, stderr, execModeRun)
}

// executionMode describes the user-visible behavior differences between
// subcommands that resolve a plan.
type executionMode struct {
	// Name is used only for diagnostics and internal labels.
	Name string

	// DryRun indicates whether the command should avoid persistent mutations that
	// represent applying changes.
	//
	// Note: even in dry-run mode, decomk may still self-update and update the
	// config repo when configured; those are considered bootstrap steps.
	DryRun bool

	// MakeFlags are passed to make before variable tuples and targets.
	MakeFlags []string

	// WriteEnv controls whether decomk writes <DECOMK_HOME>/env.sh.
	WriteEnv bool

	// LockStamps controls whether decomk takes the global stamps lock and touches
	// existing stamps before running make.
	LockStamps bool

	// Log controls whether decomk writes make output to a per-run log file.
	Log bool
}

var (
	execModePlan = executionMode{
		Name:       "plan",
		DryRun:     true,
		MakeFlags:  []string{"-n"},
		WriteEnv:   false,
		LockStamps: false,
		Log:        false,
	}
	execModeRun = executionMode{
		Name:       "run",
		DryRun:     false,
		MakeFlags:  nil,
		WriteEnv:   true,
		LockStamps: true,
		Log:        true,
	}
)

// cmdExecute is the shared implementation for plan/run.
//
// Both commands:
//   - parse flags and action args
//   - apply -C (starting directory) for relative path resolution
//   - resolve config, expand macros, and partition tokens
//   - select targets (isconf-style action args)
//   - invoke make (real or -n)
//
// The executionMode controls whether env.sh is written, whether stamp state is
// locked/touched, and whether output is captured to a per-run log file.
func cmdExecute(args []string, stdout, stderr io.Writer, mode executionMode) (int, error) {
	fs := flag.NewFlagSet("decomk "+mode.Name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f commonFlags
	addCommonFlags(fs, &f)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}
	actionArgs := fs.Args()

	if err := applyStartDir(f.startDir); err != nil {
		return 1, err
	}

	plan, err := resolvePlanFromFlags(f, stderr)
	if err != nil {
		return 1, err
	}
	if plan == nil {
		return 1, fmt.Errorf("internal error: resolvePlanFromFlags returned nil plan")
	}
	if plan.Makefile == "" {
		return 1, fmt.Errorf("no Makefile found; use -makefile to set an explicit path")
	}

	targets, targetSource := selectTargets(plan.Targets, plan.Tuples, actionArgs)

	if mode.DryRun {
		printPlan(stdout, plan, actionArgs, targets, targetSource)
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "env exports (dry-run; not written):")
		if err := writeEnvExport(stdout, plan, targets); err != nil {
			return 1, err
		}
	}

	// Ensure the stamp dir exists so make can run. This does not touch any stamp
	// files; it only ensures the directory exists.
	if err := state.EnsureDir(plan.StampDir); err != nil {
		return 1, err
	}

	var lock *state.Lock
	if mode.LockStamps {
		// Prevent concurrent stamp mutation for the container.
		lock, err = state.LockFile(state.StampsLockPath(plan.Home))
		if err != nil {
			return 1, fmt.Errorf("lock stamps: %w", err)
		}
		defer lock.Close()

		// Normalize mtime semantics once per invocation.
		if err := state.TouchExistingStamps(plan.StampDir, time.Now()); err != nil {
			return 1, fmt.Errorf("touch stamps: %w", err)
		}
	}

	if mode.WriteEnv {
		if err := writeEnvFile(plan.EnvFile, plan, targets); err != nil {
			return 1, err
		}
	}

	makeTuples, makeEnv := makeInvocation(plan, targets)

	out := stdout
	errOut := stderr
	var runLogPath string
	if mode.Log {
		// Include sub-second resolution and pid to avoid collisions when two runs start
		// close together (otherwise one run can clobber the other's log output).
		runID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + strconv.Itoa(os.Getpid())
		runLogDir, err := createRunLogDir(plan, runID, stderr)
		if err != nil {
			return 1, err
		}
		runLogPath = filepath.Join(runLogDir, "make.log")
		logFile, err := os.OpenFile(runLogPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			return 1, err
		}
		defer logFile.Close()

		out = io.MultiWriter(stdout, logFile)
		errOut = io.MultiWriter(stderr, logFile)
	}

	if mode.DryRun {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "make -n output:")
	}

	exitCode, runErr := makeexec.RunWithFlags(plan.StampDir, plan.Makefile, mode.MakeFlags, makeTuples, targets, makeEnv, out, errOut)
	if runErr != nil {
		if runLogPath != "" {
			return exitCode, fmt.Errorf("make failed (exit %d); log: %s: %w", exitCode, runLogPath, runErr)
		}
		return exitCode, fmt.Errorf("make failed (exit %d): %w", exitCode, runErr)
	}
	return 0, nil
}

// printPlan prints the human-readable plan header and resolved argv pieces.
func printPlan(w io.Writer, plan *resolvedPlan, actionArgs, targets []string, targetSource string) {
	fmt.Fprintf(w, "home: %s\n", plan.Home)
	if len(plan.WorkspaceRepos) > 0 {
		var names []string
		for _, repo := range plan.WorkspaceRepos {
			names = append(names, repo.Name)
		}
		fmt.Fprintf(w, "workspaces: %s\n", strings.Join(names, " "))
	}
	if len(plan.ContextKeys) > 0 {
		fmt.Fprintf(w, "contexts: %s\n", strings.Join(plan.ContextKeys, " "))
	}
	fmt.Fprintf(w, "config: %s\n", strings.Join(plan.ConfigPaths, ", "))
	fmt.Fprintf(w, "env: %s\n", plan.EnvFile)
	fmt.Fprintf(w, "stampDir: %s\n", plan.StampDir)
	if len(actionArgs) > 0 {
		fmt.Fprintf(w, "actionArgs: %s\n", strings.Join(actionArgs, " "))
	}
	fmt.Fprintf(w, "targetSource: %s\n", targetSource)
	if plan.Makefile != "" {
		fmt.Fprintf(w, "makefile: %s\n", plan.Makefile)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "tuples:")
	for _, t := range plan.Tuples {
		fmt.Fprintf(w, "  %s\n", t)
	}
	fmt.Fprintln(w, "targets:")
	if len(targets) == 0 {
		fmt.Fprintln(w, "  (none; make will use its default goal)")
	}
	for _, t := range targets {
		fmt.Fprintf(w, "  %s\n", t)
	}
}

// createUniqueDir creates a directory at base, adding a numeric suffix when the
// directory already exists.
//
// This is used for per-run log directories so that two decomk invocations that
// start at the same time don't clobber each other's output.
func createUniqueDir(base string) (string, error) {
	if err := state.EnsureDir(filepath.Dir(base)); err != nil {
		return "", err
	}

	dir := base
	for i := 0; ; i++ {
		if err := os.Mkdir(dir, 0o755); err != nil {
			if os.IsExist(err) {
				dir = base + "-" + strconv.Itoa(i+2)
				continue
			}
			return "", err
		}
		return dir, nil
	}
}

// createRunLogDir creates a per-run log directory and returns its path.
//
// decomk prefers to write per-run logs under plan.LogRoot. If plan.LogRoot is
// the default log root and is not writable, decomk falls back to writing logs
// under <DECOMK_HOME>/log so `decomk run` remains usable in non-root
// environments.
func createRunLogDir(plan *resolvedPlan, runID string, stderr io.Writer) (string, error) {
	base := filepath.Join(plan.LogRoot, runID)
	dir, err := createUniqueDir(base)
	if err == nil {
		return dir, nil
	}

	if plan.LogRootExplicit {
		return "", fmt.Errorf("create run log dir %s: %w", base, err)
	}

	fallbackRoot := state.LogDir(plan.Home)
	fallbackBase := filepath.Join(fallbackRoot, runID)
	fallbackDir, fallbackErr := createUniqueDir(fallbackBase)
	if fallbackErr == nil {
		fmt.Fprintf(stderr, "decomk: log dir %s not writable; falling back to %s (set -log-dir or DECOMK_LOG_DIR to override)\n", plan.LogRoot, fallbackRoot)
		return fallbackDir, nil
	}

	return "", fmt.Errorf("create run log dir: tried %s: %v; fallback %s: %v", base, err, fallbackBase, fallbackErr)
}

// applyStartDir changes the current working directory to match -C.
//
// This mirrors the common expectation of tools like make: relative paths provided
// on the command line (for example -config, -makefile, or local -tool-repo
// paths) are interpreted relative to -C.
//
// decomk also normalizes -C to an absolute path in os.Args so that if decomk
// self-updates and re-execs into a new binary, the restarted process will not
// accidentally apply a relative -C a second time (syscall.Exec preserves cwd).
func applyStartDir(startDir string) error {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return fmt.Errorf("abs -C %q: %w", startDir, err)
	}
	rewriteCFlagInOSArgs(abs)
	if err := os.Chdir(abs); err != nil {
		return fmt.Errorf("chdir -C %q: %w", startDir, err)
	}
	return nil
}

// rewriteCFlagInOSArgs rewrites any "-C" flag occurrences in os.Args to use an
// absolute path.
func rewriteCFlagInOSArgs(absStartDir string) {
	os.Args = rewriteCFlag(os.Args, absStartDir)
}

// rewriteCFlag rewrites any "-C" flag occurrences in args to use an absolute
// path.
//
// This supports both forms:
//   - "-C <dir>"
//   - "-C=<dir>"
//
// It returns a new slice so callers can use it without mutating shared state.
func rewriteCFlag(args []string, absStartDir string) []string {
	out := append([]string(nil), args...)
	for i := 1; i < len(out); i++ {
		arg := out[i]
		switch {
		case arg == "-C" && i+1 < len(out):
			out[i+1] = absStartDir
			i++
		case strings.HasPrefix(arg, "-C="):
			out[i] = "-C=" + absStartDir
		}
	}
	return out
}

const defaultWorkspacesDir = "/workspaces"

// resolveLogRoot determines where decomk should write per-run logs.
//
// Precedence:
//   - flagOverride (if non-empty)
//   - DECOMK_LOG_DIR
//   - /var/log/decomk (state.DefaultLogDir)
//
// The returned bool reports whether the result was explicitly configured via
// flag/env. Callers can use this to decide whether to fall back when the
// default isn't writable.
func resolveLogRoot(flagOverride string) (logRoot string, explicit bool, err error) {
	switch {
	case flagOverride != "":
		logRoot = flagOverride
		explicit = true
	case os.Getenv("DECOMK_LOG_DIR") != "":
		logRoot = os.Getenv("DECOMK_LOG_DIR")
		explicit = true
	default:
		logRoot = state.DefaultLogDir
		explicit = false
	}

	label := "default log dir"
	if explicit {
		if flagOverride != "" {
			label = "flag -log-dir"
		} else {
			label = "DECOMK_LOG_DIR"
		}
	}
	if !filepath.IsAbs(logRoot) {
		return "", false, fmt.Errorf("%s must be an absolute path (got %q)", label, logRoot)
	}

	return filepath.Clean(logRoot), explicit, nil
}

// resolveWorkspacesDir determines where decomk should scan for WIP workspace
// repos.
//
// Precedence:
//   - flagOverride (if non-empty)
//   - DECOMK_WORKSPACES_DIR
//   - /workspaces
func resolveWorkspacesDir(flagOverride string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if env := os.Getenv("DECOMK_WORKSPACES_DIR"); env != "" {
		return env
	}
	return defaultWorkspacesDir
}

// resolvePlanFromFlags builds a single fully-resolved plan from the user-facing
// flags.
//
// In the common devcontainer case, there may be multiple repos under
// /workspaces/* and the container should be configured based on all of them.
// decomk discovers the workspaces, selects any matching config keys, and expands
// them all into one merged set of tuples/targets.
//
// If the user explicitly sets a context (via -context or DECOMK_CONTEXT), decomk
// skips workspace discovery and expands only that context (plus DEFAULT when
// present). This makes debugging and experimentation predictable.
func resolvePlanFromFlags(f commonFlags, stderr io.Writer) (*resolvedPlan, error) {
	home, err := state.Home(f.home)
	if err != nil {
		return nil, err
	}

	logRoot, logRootExplicit, err := resolveLogRoot(f.logDir)
	if err != nil {
		return nil, err
	}

	workspacesDir := resolveWorkspacesDir(f.workspacesDir)

	// Before doing any other work, update decomk itself (isconf-style). This
	// may rebuild and re-exec into the updated binary under <home>/decomk.
	if err := selfUpdateTool(home, workspacesDir, f.toolRepo, f.verbose, stderr); err != nil {
		return nil, err
	}

	// Clone/pull the shared config repo into <home>/conf.
	if err := ensureConfRepo(home, f.confRepo, f.verbose, stderr); err != nil {
		return nil, err
	}

	explicitConfig := f.config
	if explicitConfig == "" {
		explicitConfig = os.Getenv("DECOMK_CONFIG")
	}
	if explicitConfig != "" {
		abs, err := filepath.Abs(explicitConfig)
		if err != nil {
			return nil, fmt.Errorf("abs config path %q: %w", explicitConfig, err)
		}
		explicitConfig = abs
	}

	defs, configPaths, err := loadDefs(home, explicitConfig)
	if err != nil {
		return nil, err
	}

	// If the user explicitly sets a context, do not scan workspaces.
	explicitContext := f.context
	if explicitContext == "" {
		explicitContext = os.Getenv("DECOMK_CONTEXT")
	}
	var (
		workspaceRepos []workspaceRepo
		contextKeys    []string
	)
	if explicitContext != "" {
		key, err := selectContextKey(defs, explicitContext)
		if err != nil {
			return nil, err
		}
		contextKeys = []string{key}
	} else {
		workspaceRepos, err = discoverWorkspaces(workspacesDir)
		if err != nil {
			return nil, err
		}
		contextKeys = contextKeysForWorkspaces(defs, workspaceRepos)
	}

	seed := seedTokensForContexts(defs, contextKeys)
	expanded, err := expand.ExpandTokens(expand.Defs(defs), seed, expand.Options{MaxDepth: f.maxExpDepth})
	if err != nil {
		return nil, err
	}
	tuples, targets := resolve.Partition(expanded)

	stampDir := state.StampDir(home)
	envFile := state.EnvFile(home)

	makefile := f.makefile
	if makefile != "" {
		abs, err := filepath.Abs(makefile)
		if err != nil {
			return nil, fmt.Errorf("abs makefile path %q: %w", makefile, err)
		}
		makefile = abs
	}
	if makefile == "" {
		makefile = findDefaultMakefile(home, explicitConfig)
	}
	if makefile != "" {
		abs, err := filepath.Abs(makefile)
		if err != nil {
			return nil, fmt.Errorf("abs makefile path %q: %w", makefile, err)
		}
		makefile = abs
	}
	if makefile != "" && !fileExists(makefile) {
		return nil, fmt.Errorf("makefile not found: %s", makefile)
	}

	return &resolvedPlan{
		Home:            home,
		LogRoot:         logRoot,
		LogRootExplicit: logRootExplicit,
		WorkspaceRepos:  workspaceRepos,
		ContextKeys:     seed,
		ConfigPaths:     configPaths,
		StampDir:        stampDir,
		EnvFile:         envFile,
		Makefile:        makefile,
		Expanded:        expanded,
		Tuples:          tuples,
		Targets:         targets,
	}, nil
}

// workspaceRepo describes a workspace repo directory that may drive context
// selection.
type workspaceRepo struct {
	Root      string // absolute path to the repo root (workspace root)
	Name      string // basename of Root
	OriginURL string // git remote.origin.url, if available
	OwnerRepo string // parsed "owner/repo" when possible (may be empty)
	RepoName  string // parsed repo name when possible (falls back to Name)
}

// discoverWorkspaces finds candidate workspaces under workspacesDir.
//
// This intentionally does not recurse: the expected layout is one checkout per
// child directory (e.g., /workspaces/<repo>).
//
// A directory does not need to be a git repo to be considered a "workspace" for
// identity selection; if git metadata is missing, decomk falls back to using the
// directory basename as an identity hint.
func discoverWorkspaces(workspacesDir string) ([]workspaceRepo, error) {
	if workspacesDir == "" {
		workspacesDir = defaultWorkspacesDir
	}
	entries, err := os.ReadDir(workspacesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var repos []workspaceRepo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		root := filepath.Join(workspacesDir, name)
		repos = append(repos, inspectWorkspaceRepo(root))
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Root < repos[j].Root })
	return repos, nil
}

// isGitRepoRoot reports whether dir looks like a git repo root.
//
// For decomk, a "workspace repo" is expected to be a normal checkout where
// ".git" exists (as a directory or file).
func isGitRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// inspectWorkspaceRepo derives identity hints from a workspace repo root.
//
// If git metadata is unavailable (e.g., no origin remote), fields fall back to
// stable filesystem-derived values.
func inspectWorkspaceRepo(root string) workspaceRepo {
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}

	repo := workspaceRepo{
		Root: root,
		Name: filepath.Base(root),
	}

	origin, err := gitOutput(root, "config", "--get", "remote.origin.url")
	if err == nil {
		repo.OriginURL = origin
	}

	ownerRepo, repoName := parseOwnerRepo(repo.OriginURL)
	repo.OwnerRepo = ownerRepo
	if repoName != "" {
		repo.RepoName = repoName
	} else {
		repo.RepoName = repo.Name
	}
	return repo
}

// parseOwnerRepo attempts to derive an "owner/repo" identifier from a git
// origin URL.
//
// Supported input shapes include (non-exhaustive):
//   - https://host/owner/repo(.git)
//   - ssh://git@host/owner/repo(.git)
//   - git@host:owner/repo(.git)
//   - owner/repo(.git)
//   - /some/path/to/repo(.git) (best-effort; last two path segments)
func parseOwnerRepo(originURL string) (ownerRepo, repoName string) {
	s := strings.TrimSpace(originURL)
	if s == "" {
		return "", ""
	}
	s = strings.TrimSuffix(s, ".git")

	// Handle scp-like syntax: git@host:owner/repo
	if !strings.Contains(s, "://") {
		if i := strings.LastIndex(s, ":"); i != -1 && i+1 < len(s) {
			after := s[i+1:]
			if strings.Contains(after, "/") {
				s = after
			}
		}
	} else {
		// For standard URLs, use the path portion.
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			s = strings.TrimPrefix(u.Path, "/")
		}
	}

	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return "", ""
	}
	repoName = parts[len(parts)-1]
	if len(parts) >= 2 {
		owner := parts[len(parts)-2]
		ownerRepo = owner + "/" + repoName
	}
	return ownerRepo, repoName
}

// defaultToolRepoURL is the upstream repo URL used when decomk can't infer a
// better source for its own tool repo clone.
//
// This is only used for the initial clone. Once the clone exists, subsequent
// updates use the clone's configured remote.
const defaultToolRepoURL = "https://github.com/stevegt/decomk"

// selfUpdateTool performs an isconf-style self-update:
//   - ensure a decomk tool repo clone exists at <home>/decomk
//   - git pull --ff-only to update it
//   - go build an updated decomk binary from that clone
//   - re-exec into the updated binary when appropriate
//
// This keeps the workspace repos clean and ensures that decomk's behavior tracks
// the latest pulled source.
//
// If updates require network access and the pull fails, this returns an error.
func selfUpdateTool(home, workspacesDir, repoURL string, verbose bool, stderr io.Writer) error {
	if repoURL == "" {
		repoURL = os.Getenv("DECOMK_TOOL_REPO")
	}

	// Serialize clone/pull/build operations so concurrent decomk invocations can't
	// corrupt the tool working tree or clobber the built binary.
	lock, err := state.LockFile(state.ToolLockPath(home))
	if err != nil {
		return fmt.Errorf("lock tool repo: %w", err)
	}

	changed, err := ensureToolRepo(home, workspacesDir, repoURL, verbose, stderr)
	if err != nil {
		_ = lock.Close()
		return err
	}

	binPath := state.ToolBinPath(home)
	if changed || !fileExists(binPath) {
		if err := buildToolBinary(home, verbose, stderr); err != nil {
			_ = lock.Close()
			return err
		}
	}

	// Decide whether to re-exec into the tool binary we just built. We do this
	// when:
	//   - we're not currently executing that binary (common for "go run" or
	//     a system-installed decomk), or
	//   - we pulled new source and rebuilt a new binary at the same path.
	exe, err := os.Executable()
	if err != nil {
		_ = lock.Close()
		return fmt.Errorf("find current executable: %w", err)
	}

	needExec := exe != binPath || changed
	if !needExec {
		return lock.Close()
	}

	if verbose {
		fmt.Fprintf(stderr, "decomk: re-exec into %s\n", binPath)
	}

	// Release the lock before exec so the new process can lock/pull/build again
	// if needed.
	if err := lock.Close(); err != nil {
		return err
	}

	argv := append([]string{binPath}, os.Args[1:]...)
	return syscall.Exec(binPath, argv, os.Environ())
}

// ensureToolRepo ensures the decomk tool repo clone exists at <home>/decomk and
// is up to date.
//
// If repoURL is empty, decomk will try to infer a suitable URL from a sibling
// workspace checkout (e.g., "/workspaces/decomk"), falling back to
// defaultToolRepoURL.
//
// It returns changed=true if the clone was newly created or if git pull changed
// HEAD.
func ensureToolRepo(home, workspacesDir, repoURL string, verbose bool, stderr io.Writer) (changed bool, err error) {
	toolDir := state.ToolDir(home)

	stat, err := os.Stat(toolDir)
	switch {
	case err == nil:
		if !stat.IsDir() {
			return false, fmt.Errorf("tool repo path exists but is not a directory: %s", toolDir)
		}
		ok, err := isGitWorkTree(toolDir)
		if err != nil {
			return false, fmt.Errorf("check tool repo git state: %w", err)
		}
		if !ok {
			return false, fmt.Errorf("tool repo directory exists but is not a git work tree: %s", toolDir)
		}

		origin, _ := gitOutput(toolDir, "config", "--get", "remote.origin.url")
		if repoURL != "" && origin != "" && origin != repoURL {
			return false, fmt.Errorf("tool repo origin URL mismatch: want %q, got %q (dir %s)", repoURL, origin, toolDir)
		}

		before, _ := gitOutput(toolDir, "rev-parse", "HEAD")
		if verbose {
			fmt.Fprintf(stderr, "decomk: updating tool repo in %s\n", toolDir)
		}
		if err := runGit(stderr, toolDir, "pull", "--ff-only"); err != nil {
			return false, fmt.Errorf("update tool repo: %w", err)
		}
		after, _ := gitOutput(toolDir, "rev-parse", "HEAD")
		return before != "" && after != "" && before != after, nil

	case os.IsNotExist(err):
		cloneURL := repoURL
		if cloneURL == "" {
			cloneURL = inferToolRepoURL(workspacesDir)
		}
		if cloneURL == "" {
			cloneURL = defaultToolRepoURL
		}

		if err := state.EnsureDir(home); err != nil {
			return false, err
		}
		if verbose {
			fmt.Fprintf(stderr, "decomk: cloning tool repo into %s\n", toolDir)
		}
		cmd := exec.Command("git", "clone", cloneURL, toolDir)
		cmd.Stdout = stderr
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return false, fmt.Errorf("git clone tool repo: %w", err)
		}
		return true, nil

	default:
		return false, fmt.Errorf("stat tool repo dir %q: %w", toolDir, err)
	}
}

// inferToolRepoURL tries to derive the decomk tool repo URL from a sibling
// workspace checkout.
//
// This is a pragmatic devcontainer-friendly default: multi-repo workspaces often
// include a WIP clone at "/workspaces/decomk". If it exists, we prefer its
// origin URL, falling back to the local path itself (which works as a git clone
// source).
func inferToolRepoURL(workspacesDir string) string {
	if workspacesDir == "" {
		workspacesDir = defaultWorkspacesDir
	}
	candidate := filepath.Join(workspacesDir, "decomk")
	if !isGitRepoRoot(candidate) {
		return ""
	}
	origin, err := gitOutput(candidate, "config", "--get", "remote.origin.url")
	if err == nil && origin != "" {
		return origin
	}
	return candidate
}

// buildToolBinary builds the decomk binary from the tool repo clone into
// <home>/decomk/bin/decomk.
func buildToolBinary(home string, verbose bool, stderr io.Writer) error {
	toolDir := state.ToolDir(home)
	binPath := state.ToolBinPath(home)

	if err := state.EnsureDir(filepath.Dir(binPath)); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(stderr, "decomk: building %s\n", binPath)
	}
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/decomk")
	cmd.Dir = toolDir
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build decomk: %w", err)
	}
	return nil
}

// ensureConfRepo ensures the shared config repo clone exists under <home>/conf.
//
// If repoURL is empty (and DECOMK_CONF_REPO is unset), this function does
// nothing.
//
// Behavior:
//   - If <home>/conf does not exist: git clone repoURL into it.
//   - If <home>/conf exists and is a git repo: git pull --ff-only.
func ensureConfRepo(home, repoURL string, verbose bool, stderr io.Writer) error {
	if repoURL == "" {
		repoURL = os.Getenv("DECOMK_CONF_REPO")
	}
	if repoURL == "" {
		return nil
	}

	// Serialize clone/pull operations so concurrent decomk invocations can't
	// corrupt the working tree.
	lock, err := state.LockFile(state.ConfLockPath(home))
	if err != nil {
		return fmt.Errorf("lock config repo: %w", err)
	}
	defer lock.Close()

	confDir := state.ConfDir(home)

	stat, err := os.Stat(confDir)
	switch {
	case err == nil:
		if !stat.IsDir() {
			return fmt.Errorf("config repo path exists but is not a directory: %s", confDir)
		}
		ok, err := isGitWorkTree(confDir)
		if err != nil {
			return fmt.Errorf("check config repo git state: %w", err)
		}
		if !ok {
			return fmt.Errorf("config repo directory exists but is not a git work tree: %s", confDir)
		}

		origin, _ := gitOutput(confDir, "config", "--get", "remote.origin.url")
		if origin != "" && origin != repoURL {
			return fmt.Errorf("config repo origin URL mismatch: want %q, got %q (dir %s)", repoURL, origin, confDir)
		}

		if verbose {
			fmt.Fprintf(stderr, "decomk: updating config repo in %s\n", confDir)
		}
		if err := runGit(stderr, confDir, "pull", "--ff-only"); err != nil {
			return fmt.Errorf("update config repo: %w", err)
		}
		return nil

	case os.IsNotExist(err):
		if err := state.EnsureDir(home); err != nil {
			return err
		}
		if verbose {
			fmt.Fprintf(stderr, "decomk: cloning config repo into %s\n", confDir)
		}
		cmd := exec.Command("git", "clone", repoURL, confDir)
		cmd.Stdout = stderr
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone config repo: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("stat config repo dir %q: %w", confDir, err)
	}
}

// isGitWorkTree reports whether dir is inside a git working tree.
func isGitWorkTree(dir string) (bool, error) {
	out, err := gitOutput(dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		// git returns a non-zero exit status if dir is not a repo.
		return false, nil
	}
	return strings.TrimSpace(out) == "true", nil
}

// gitOutput runs "git -C dir args..." and returns stdout as a trimmed string.
func gitOutput(dir string, args ...string) (string, error) {
	a := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", a...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runGit runs "git -C dir args..." and streams stdout/stderr to w.
func runGit(w io.Writer, dir string, args ...string) error {
	a := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", a...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// loadDefs loads decomk.conf trees from multiple sources and merges them.
//
// Precedence is "last wins" (higher precedence overrides lower):
//
// Future: extend precedence beyond these sources (e.g., per-owner/per-org
// defaults, container-image defaults, etc.) while keeping the model auditable.
//
//  1. config repo decomk.conf (lowest; optional)
//  2. explicit -config / DECOMK_CONFIG (highest; optional)
//
// Each source is loaded via contexts.LoadTree so it can also include a sibling
// decomk.d/*.conf directory.
func loadDefs(home, explicitConfig string) (defs contexts.Defs, paths []string, err error) {
	// Precedence: config repo (lowest) -> explicit override (highest).
	var sources []string

	if configRepo, ok := configRepoConfigPath(home); ok {
		sources = append(sources, configRepo)
	}

	if explicitConfig != "" {
		if !fileExists(explicitConfig) {
			return nil, nil, fmt.Errorf("config file not found: %s", explicitConfig)
		}
		sources = append(sources, explicitConfig)
	}

	if len(sources) == 0 {
		tried := append([]string(nil), configRepoConfigCandidates(home)...)
		return nil, nil, fmt.Errorf("no config found; tried %s; set -config/DECOMK_CONFIG or -conf-repo/DECOMK_CONF_REPO", strings.Join(tried, ", "))
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

// configRepoConfigCandidates returns candidate decomk.conf paths inside the
// config repo clone.
//
// The config repo is expected to keep decomk.conf at the root of the clone
// directory (<DECOMK_HOME>/conf/decomk.conf). decomk intentionally does not
// search alternate layouts (for example a nested etc/ directory) so that the
// precedence model stays simple and predictable.
func configRepoConfigCandidates(home string) []string {
	return []string{
		filepath.Join(state.ConfDir(home), "decomk.conf"),
	}
}

// configRepoConfigPath returns the first existing config repo decomk.conf path.
func configRepoConfigPath(home string) (string, bool) {
	for _, p := range configRepoConfigCandidates(home) {
		if fileExists(p) {
			return p, true
		}
	}
	return "", false
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

// contextKeysForWorkspaces selects at most one non-DEFAULT context key for each
// discovered workspace.
//
// This helper is intentionally tolerant: if a workspace has no matching stanza
// in defs, it contributes nothing. This mirrors isconf's behavior of always
// applying DEFAULT and optionally applying host-specific stanzas only when they
// exist.
func contextKeysForWorkspaces(defs contexts.Defs, repos []workspaceRepo) []string {
	seen := make(map[string]bool)
	var keys []string
	for _, repo := range repos {
		var chosen string
		for _, c := range []string{repo.OwnerRepo, repo.RepoName, repo.Name} {
			if c == "" {
				continue
			}
			if _, ok := defs[c]; ok {
				chosen = c
				break
			}
		}
		if chosen == "" || chosen == "DEFAULT" {
			continue
		}
		if seen[chosen] {
			continue
		}
		seen[chosen] = true
		keys = append(keys, chosen)
	}
	return keys
}

// seedTokensForContexts builds the initial macro token list to expand.
//
// It always includes DEFAULT first when present, then includes each provided
// context key in order, deduplicating along the way.
func seedTokensForContexts(defs contexts.Defs, contextKeys []string) []string {
	seen := make(map[string]bool)
	var seed []string
	add := func(key string) {
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		seed = append(seed, key)
	}

	if _, ok := defs["DEFAULT"]; ok {
		add("DEFAULT")
	}
	for _, key := range contextKeys {
		add(key)
	}
	return seed
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// computedVars returns decomk-owned computed exports/variables for this plan.
//
// These variables are always defined by decomk and must not be overridden by
// config-provided tuples, because other processes (and Makefile recipes) rely on
// them to describe decomk's actual execution environment.
func computedVars(plan *resolvedPlan, targets []string) map[string]string {
	var workspaces []string
	for _, repo := range plan.WorkspaceRepos {
		workspaces = append(workspaces, repo.Name)
	}
	return map[string]string{
		"DECOMK_HOME":       plan.Home,
		"DECOMK_STAMPDIR":   plan.StampDir,
		"DECOMK_WORKSPACES": strings.Join(workspaces, " "),
		"DECOMK_CONTEXTS":   strings.Join(plan.ContextKeys, " "),
		"DECOMK_PACKAGES":   strings.Join(targets, " "),
	}
}

// selectTargets determines which make targets decomk should pass on argv for a
// given plan.
//
// Semantics are intentionally isconf-like:
//   - If actionArgs is non-empty, actionArgs drive the selection and config
//     target tokens are ignored.
//   - Each arg is treated as either:
//   - an action variable name (when it matches a resolved tuple variable),
//     in which case that variable's value is split on whitespace into targets, or
//   - a literal make target (fallback).
//   - If actionArgs is empty, decomk preserves its historical behavior:
//   - run config target tokens if present, else
//   - default to INSTALL (if defined), else
//   - pass no targets (make's default goal).
func selectTargets(configTargets, tuples, actionArgs []string) (targets []string, source string) {
	effective := effectiveTupleValues(tuples)
	if len(actionArgs) > 0 {
		return targetsFromActionArgs(actionArgs, effective), "actionArgs"
	}
	if len(configTargets) > 0 {
		return append([]string(nil), configTargets...), "configTargets"
	}
	if installTargets, ok := effective["INSTALL"]; ok {
		if split := splitTargetList(installTargets); len(split) > 0 {
			return split, "defaultINSTALL"
		}
	}
	return nil, "makeDefaultGoal"
}

// effectiveTupleValues returns the "last wins" values for NAME=value tuples.
//
// This mirrors make's command-line variable precedence: if the same variable
// name appears multiple times on argv, the last assignment wins.
func effectiveTupleValues(tuples []string) map[string]string {
	out := make(map[string]string, len(tuples))
	for _, t := range tuples {
		k, v, ok := resolve.SplitTuple(t)
		if !ok {
			continue
		}
		out[k] = v
	}
	return out
}

// targetsFromActionArgs interprets each action arg as either a tuple-variable
// name (expanding to a whitespace-separated target list) or a literal target.
func targetsFromActionArgs(actionArgs []string, tupleValues map[string]string) []string {
	var targets []string
	for _, arg := range actionArgs {
		if v, ok := tupleValues[arg]; ok {
			targets = append(targets, splitTargetList(v)...)
			continue
		}
		targets = append(targets, arg)
	}
	return targets
}

// splitTargetList splits a tuple value into a target list.
//
// We intentionally treat the value as plain text and split on whitespace. Target
// names containing whitespace are technically possible in make, but they are
// uncommon and awkward in practice, and decomk's isconf-style action variables
// are expected to contain conventional target names.
func splitTargetList(value string) []string {
	return strings.Fields(value)
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
// repo" (under <DECOMK_HOME>/conf). Repo-local decomk.conf is intended as an
// overlay, not as the canonical source of make recipes.
//
// Selection order (first match wins):
//  1. sibling of explicitConfig (if non-empty)
//  2. <DECOMK_HOME>/conf/Makefile
func findDefaultMakefile(home, explicitConfig string) string {
	if explicitConfig != "" {
		candidate := filepath.Join(filepath.Dir(explicitConfig), "Makefile")
		if fileExists(candidate) {
			return candidate
		}
	}
	candidate := filepath.Join(state.ConfDir(home), "Makefile")
	if fileExists(candidate) {
		return candidate
	}
	return ""
}

// makeInvocation returns the tuple list and environment slice for invoking make.
//
// This is shared by plan (make -n) and run (real make) so both paths agree on
// which computed variables are exported.
func makeInvocation(plan *resolvedPlan, targets []string) (tuples []string, env []string) {
	tuples = append([]string(nil), plan.Tuples...)

	// Append computed variables last so they override any config-provided tuples
	// with the same NAME. This mirrors isconf's "last wins" make argv behavior and
	// ensures decomk-owned variables are trustworthy.
	//
	// Note: some values contain spaces (e.g. DECOMK_PACKAGES). This is safe: argv
	// elements are not re-split by spaces when exec'd.
	cv := computedVars(plan, targets)
	for _, name := range []string{"DECOMK_HOME", "DECOMK_STAMPDIR", "DECOMK_WORKSPACES", "DECOMK_CONTEXTS", "DECOMK_PACKAGES"} {
		if v, ok := cv[name]; ok {
			tuples = append(tuples, name+"="+v)
		}
	}

	env = withEnv(os.Environ(), cv)
	return tuples, env
}

// writeEnvFile writes the shell-friendly env export file that captures the
// resolved configuration for the run.
//
// This file is intentionally simple: it is designed to be sourced by scripts
// and nested make invocations without requiring eval.
func writeEnvFile(path string, plan *resolvedPlan, targets []string) error {
	if err := state.EnsureParentDir(path); err != nil {
		return err
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	if err := writeEnvExport(f, plan, targets); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeEnvExport writes the full env export file content to w.
//
// The output format is a POSIX-shell-friendly sequence of "export NAME='value'"
// lines, optionally preceded by comment lines. It is safe to "source" this file
// in a shell or make recipe.
func writeEnvExport(w io.Writer, plan *resolvedPlan, targets []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(w, "# generated by decomk; do not edit\n")
	fmt.Fprintf(w, "# time: %s\n", now)
	if len(plan.ContextKeys) > 0 {
		fmt.Fprintf(w, "# contexts: %s\n", strings.Join(plan.ContextKeys, " "))
	}
	if len(plan.WorkspaceRepos) > 0 {
		var names []string
		for _, repo := range plan.WorkspaceRepos {
			names = append(names, repo.Name)
		}
		fmt.Fprintf(w, "# workspaces: %s\n", strings.Join(names, " "))
	}
	fmt.Fprintf(w, "# config: %s\n", strings.Join(plan.ConfigPaths, ", "))
	fmt.Fprintln(w)

	// Export config-provided tuples first.
	//
	// computedVars are emitted later and override any config-provided entries with
	// the same variable name.
	for _, t := range plan.Tuples {
		k, v, ok := resolve.SplitTuple(t)
		if !ok {
			continue
		}
		writeExport(w, k, v)
	}

	// Export computed helpers for recipes/scripts last so they override any
	// config-provided values.
	cv := computedVars(plan, targets)
	for _, name := range []string{"DECOMK_HOME", "DECOMK_STAMPDIR", "DECOMK_WORKSPACES", "DECOMK_CONTEXTS", "DECOMK_PACKAGES"} {
		writeExport(w, name, cv[name])
	}
	return nil
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
