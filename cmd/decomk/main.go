// Command decomk is an isconf-inspired bootstrap wrapper for devcontainers.
//
// MVP responsibilities:
//   - Load decomk.conf (and optional decomk.d/*.conf).
//   - Expand macros into a tuple-only policy set (VAR=value assignments).
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
	"os/user"
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
	"github.com/stevegt/envi"
)

const defaultVersion = "dev"

// decomkVersion is the CLI version string printed by `decomk version`.
//
// Build pipelines can override this via `-ldflags "-X main.decomkVersion=<value>"`.
var decomkVersion = defaultVersion

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

// run is the CLI entrypoint. It returns an exit code (like main) rather than
// calling os.Exit directly, which makes it easy to test in the future.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		if err := writeLine(stderr, usage()); err != nil {
			return 1
		}
		return 2
	}

	switch args[1] {
	case "-h", "-help", "--help", "help":
		if err := writeLine(stdout, usage()); err != nil {
			return 1
		}
		return 0
	case "version":
		code, err := cmdVersion(args[2:], stdout, stderr)
		if err != nil {
			if printErr := writeLine(stderr, err.Error()); printErr != nil {
				return 1
			}
			return code
		}
		return code
	case "init":
		// Intent: Make stage-0 devcontainer bootstrap scaffolding first-class in
		// decomk so new repos can be bootstrapped consistently without manual file
		// copy/paste.
		// Source: DI-001-20260311-161825 (TODO/001)
		code, err := cmdInit(args[2:], stdout, stderr)
		if err != nil {
			if printErr := writeLine(stderr, err.Error()); printErr != nil {
				return 1
			}
			return code
		}
		return code
	case "init-conf":
		// Intent: Let operators bootstrap a shared decomk config repo (including
		// a starter producer devcontainer) from embedded templates instead of
		// hand-authoring the initial conf tree.
		// Source: DI-013-20260422-110500 (TODO/013)
		code, err := cmdInitConf(args[2:], stdout, stderr)
		if err != nil {
			if printErr := writeLine(stderr, err.Error()); printErr != nil {
				return 1
			}
			return code
		}
		return code
	case "plan":
		code, err := cmdPlan(args[2:], stdout, stderr)
		if err != nil {
			if printErr := writeLine(stderr, err.Error()); printErr != nil {
				return 1
			}
			return code
		}
		return code
	case "run":
		code, err := cmdRun(args[2:], stdout, stderr)
		if err != nil {
			if printErr := writeLine(stderr, err.Error()); printErr != nil {
				return 1
			}
			return code
		}
		return code
	case "checkpoint":
		// Intent: Provide first-class checkpoint lifecycle commands (`build`,
		// `push`, `tag`) directly in decomk so operators can run one canonical CLI
		// flow instead of ad-hoc external scripts.
		// Source: DI-011-20260420-162554 (TODO/011)
		code, err := cmdCheckpoint(args[2:], stdout, stderr)
		if err != nil {
			if printErr := writeLine(stderr, err.Error()); printErr != nil {
				return 1
			}
			return code
		}
		return code
	default:
		if err := writeLine(stderr, "unknown command:", args[1]); err != nil {
			return 1
		}
		if err := writeLine(stderr, usage()); err != nil {
			return 1
		}
		return 2
	}
}

// writeLine writes one line to w and reports any write failure.
//
// Intent: Route all user-facing writes through checked helpers so decomk never
// silently drops output errors and continues on a broken stream.
// Source: DI-008-20260412-122157 (TODO/008)
func writeLine(w io.Writer, values ...any) error {
	_, err := fmt.Fprintln(w, values...)
	return err
}

// writeFormat writes formatted output to w and reports any write failure.
func writeFormat(w io.Writer, format string, values ...any) error {
	_, err := fmt.Fprintf(w, format, values...)
	return err
}

// usage returns the top-level help text.
func usage() string {
	return `decomk - devcontainer bootstrap wrapper around make

Usage:
  decomk <command> [flags] [ARGS...]

Commands:
  version  Print decomk CLI version string
  init       Install .devcontainer templates for decomk stage-0 bootstrap
  init-conf  Install shared conf-repo starter templates (decomk.conf + Makefile + producer devcontainer)
  plan    Print resolved tuples/targets + env exports; run make -n (dry-run); do not write env export file
  run     Resolve, write env export file, and run make in the stamp dir
  checkpoint  Build/push/tag checkpoint images for shared updateContent setup

ARGS (required for plan/run):
  Positional args are interpreted isconf-style:
    - If an arg matches a resolved tuple variable name (e.g. INSTALL), its value
      is split on whitespace to produce make targets.
    - Otherwise, the arg is treated as a literal make target name.
`
}

// cmdVersion prints the CLI version string and validates version-specific args.
//
// Intent: Provide a stable machine-readable version surface for scripts and
// operators without requiring `-h` parsing or external package metadata lookup.
// Source: DI-001-20260423-045924 (TODO/001)
func cmdVersion(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("decomk version", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}
	if len(fs.Args()) != 0 {
		return 2, fmt.Errorf("version does not accept positional args: %q", strings.Join(fs.Args(), " "))
	}
	if err := writeLine(stdout, decomkVersion); err != nil {
		return 1, err
	}
	return 0, nil
}

// commonFlags are the shared flags for subcommands that resolve a context.
type commonFlags struct {
	home          string
	logDir        string
	makeAsRoot    bool
	startDir      string
	workspacesDir string
	context       string
	config        string
	makefile      string
	verbose       bool
	maxExpDepth   int
}

// addCommonFlags defines flags shared by plan/run.
func addCommonFlags(fs *flag.FlagSet, f *commonFlags) {
	fs.StringVar(&f.home, "home", "", "decomk home directory (overrides DECOMK_HOME)")
	fs.StringVar(&f.logDir, "log-dir", "", "per-run log root directory (absolute path; overrides DECOMK_LOG_DIR; default /var/log/decomk)")
	fs.BoolVar(&f.makeAsRoot, "make-as-root", f.makeAsRoot, "run make as root (default true; overrides DECOMK_MAKE_AS_ROOT; when decomk is non-root this uses passwordless sudo -n; set -make-as-root=false to run make as the current user)")
	fs.StringVar(&f.startDir, "C", ".", "starting directory (like make -C)")
	fs.StringVar(&f.workspacesDir, "workspaces", "", "workspaces root directory to scan (overrides DECOMK_WORKSPACES_DIR; default /workspaces)")
	fs.StringVar(&f.context, "context", "", "context key override (also DECOMK_CONTEXT)")
	fs.StringVar(&f.config, "config", "", "config file path override (also DECOMK_CONFIG)")
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
	//
	// Intent: Prefer conventional system log locations by default while keeping
	// "decomk run" reliable in non-root environments via a default-only fallback.
	// Source: DI-005-20260309-172359 (TODO/005)
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
// Note: plan intentionally avoids bootstrap clone/pull side effects; those are
// expected to be handled by stage-0 lifecycle tooling (for example
// `.devcontainer/decomk-stage0.sh` hooks in devcontainers).
//
// plan may still create <DECOMK_HOME>/stamps if it does not exist (so make -n
// can run).
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
	// Note: dry-run mode avoids bootstrap clone/pull side effects.
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
func cmdExecute(args []string, stdout, stderr io.Writer, mode executionMode) (exitCode int, retErr error) {
	fs := flag.NewFlagSet("decomk "+mode.Name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f commonFlags

	// Intent: Default to running make as root (via passwordless sudo -n when
	// decomk is non-root) so Makefile recipes can install system packages without
	// embedding sudo and without prompting for a password.
	// Source: DI-001-20260309-172358 (TODO/001)
	f.makeAsRoot = envi.Bool("DECOMK_MAKE_AS_ROOT", true)

	addCommonFlags(fs, &f)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}
	actionArgs := fs.Args()
	// Intent: Require explicit action selection for both plan and run so decomk
	// does not silently fall back to config-derived/no-arg target behavior.
	// Source: DI-004-20260422-193652 (TODO/004)
	if len(actionArgs) == 0 {
		return 2, fmt.Errorf("decomk %s requires at least one action arg", mode.Name)
	}

	// makeAsRoot is an execution concern (how decomk invokes make), not part of
	// the resolved config/expansion model. Keep it out of resolvedPlan so plan
	// resolution stays purely about "which tokens/tuples/targets" rather than
	// "how to execute them".
	//
	// Intent: Keep plan resolution deterministic and reusable (plan/run) while
	// letting callers choose make privilege mode independently.
	// Source: DI-001-20260309-172358 (TODO/001)
	makeAsRoot := f.makeAsRoot

	if err := applyStartDir(f.startDir); err != nil {
		return 1, err
	}

	plan, err := resolvePlanFromFlags(f)
	if err != nil {
		return 1, err
	}
	if plan == nil {
		return 1, fmt.Errorf("internal error: resolvePlanFromFlags returned nil plan")
	}
	if plan.Makefile == "" {
		return 1, fmt.Errorf("no Makefile found; use -makefile to set an explicit path")
	}

	// Intent: Resolve passthrough tuples and build one canonical env tuple stream
	// once per invocation so env.sh and make receive the same effective values.
	// Source: DI-001-20260313-224000 (TODO/001)
	incomingEnvList := os.Environ()
	incomingEnv := envMapFromList(incomingEnvList)
	resolvedTuples, err := resolveTuplePassThroughs(plan.Tuples, incomingEnv)
	if err != nil {
		return 1, err
	}
	plan.Tuples = resolvedTuples

	targets, targetSource := selectTargets(plan.Tuples, actionArgs)
	cookedTuples := canonicalEnvTuples(plan, targets, makeAsRoot, incomingEnv)

	if mode.DryRun {
		if err := warnIfRunRequiresPasswordlessSudo(makeAsRoot, stderr); err != nil {
			return 1, err
		}
	}

	makeCmd, makeUsesSudo, err := resolveMakeCommand(makeAsRoot, mode.DryRun)
	if err != nil {
		return 1, err
	}

	if mode.DryRun {
		if err := printPlan(stdout, plan, actionArgs, targets, targetSource); err != nil {
			return 1, err
		}
		if err := writeLine(stdout); err != nil {
			return 1, err
		}
		if err := writeLine(stdout, "env exports (dry-run; not written):"); err != nil {
			return 1, err
		}
		if err := writeEnvExport(stdout, plan, cookedTuples); err != nil {
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
		// Intent: Preserve close errors from deferred lock release so decomk never
		// drops lock lifecycle failures.
		// Source: DI-008-20260412-122157 (TODO/008)
		defer func() {
			if lock == nil {
				return
			}
			if closeErr := lock.Close(); closeErr != nil {
				wrapped := fmt.Errorf("close stamps lock: %w", closeErr)
				if retErr == nil {
					retErr = wrapped
					if exitCode == 0 {
						exitCode = 1
					}
					return
				}
				retErr = errors.Join(retErr, wrapped)
			}
		}()

		// When make runs as root via sudo, it may leave root-owned stamps behind.
		// Fix that before touching stamps so subsequent runs stay idempotent.
		// Intent: Keep stamp ownership stable even when make runs as root.
		// Source: DI-001-20260309-172358 (TODO/001)
		if makeUsesSudo {
			if err := chownTreeToCurrentUser(plan.StampDir); err != nil {
				return 1, fmt.Errorf("chown stamp dir: %w", err)
			}
		}

		// Normalize mtime semantics once per invocation.
		if err := state.TouchExistingStamps(plan.StampDir, time.Now()); err != nil {
			return 1, fmt.Errorf("touch stamps: %w", err)
		}
	}

	if mode.WriteEnv {
		if err := writeEnvFile(plan.EnvFile, plan, cookedTuples); err != nil {
			return 1, err
		}
	}

	makeTuples, makeEnv := makeInvocation(incomingEnvList, cookedTuples)

	out := stdout
	errOut := stderr
	var runLogPath string
	var logFile *os.File
	if mode.Log {
		// Include sub-second resolution and pid to avoid collisions when two runs start
		// close together (otherwise one run can clobber the other's log output).
		runID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + strconv.Itoa(os.Getpid())
		runLogDir, err := createRunLogDir(plan, runID, stderr)
		if err != nil {
			return 1, err
		}
		runLogPath = filepath.Join(runLogDir, "make.log")
		logFile, err = os.OpenFile(runLogPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			return 1, err
		}
		// Intent: Preserve deferred log close failures so successful runs don't hide
		// file descriptor or fs-sync problems in audit logs.
		// Source: DI-008-20260412-122157 (TODO/008)
		defer func() {
			if logFile == nil {
				return
			}
			if closeErr := logFile.Close(); closeErr != nil {
				wrapped := fmt.Errorf("close run log file %s: %w", runLogPath, closeErr)
				if retErr == nil {
					retErr = wrapped
					if exitCode == 0 {
						exitCode = 1
					}
					return
				}
				retErr = errors.Join(retErr, wrapped)
			}
		}()

		out = io.MultiWriter(stdout, logFile)
		errOut = io.MultiWriter(stderr, logFile)
	}

	// Makefile recipes that drop privileges (runuser/su) typically need a
	// non-empty username. Warn early if we can't determine it so users aren't
	// surprised by a confusing "unknown user" failure during make.
	//
	// Intent: Provide a clear, early signal when user-scoped recipe patterns are
	// likely to fail in root-make mode.
	// Source: DI-001-20260309-172358 (TODO/001)
	devUser := resolveDevUser()
	makeUser := resolveMakeUser(makeAsRoot, devUser)
	if makeUser == "root" && devUser == "" {
		if err := writeLine(errOut, "decomk: warning: DECOMK_DEV_USER is empty; Makefile recipes that drop privileges (runuser/su) may fail"); err != nil {
			return 1, err
		}
	}

	if mode.DryRun {
		if err := writeLine(stdout); err != nil {
			return 1, err
		}
		if err := writeLine(stdout, "make -n output:"); err != nil {
			return 1, err
		}
	}

	exitCode, runErr := makeexec.RunWithFlagsCommand(plan.StampDir, plan.Makefile, makeCmd, mode.MakeFlags, makeTuples, targets, makeEnv, out, errOut)
	if makeUsesSudo {
		// Make may have created or updated stamp files as root. Return ownership to
		// the dev user so future runs can touch/check stamps without sudo.
		// Intent: Avoid leaving root-owned state behind after a root-make run.
		// Source: DI-001-20260309-172358 (TODO/001)
		if chownErr := chownTreeToCurrentUser(plan.StampDir); chownErr != nil {
			if runErr != nil {
				if runLogPath != "" {
					return exitCode, fmt.Errorf("make failed (exit %d); log: %s: %w; additionally failed to chown stamp dir back to user: %v", exitCode, runLogPath, runErr, chownErr)
				}
				return exitCode, fmt.Errorf("make failed (exit %d): %w; additionally failed to chown stamp dir back to user: %v", exitCode, runErr, chownErr)
			}
			return 1, fmt.Errorf("make succeeded but failed to chown stamp dir back to user: %w", chownErr)
		}
	}
	if runErr != nil {
		if runLogPath != "" {
			return exitCode, fmt.Errorf("make failed (exit %d); log: %s: %w", exitCode, runLogPath, runErr)
		}
		return exitCode, fmt.Errorf("make failed (exit %d): %w", exitCode, runErr)
	}
	return 0, nil
}

// printPlan prints the human-readable plan header and resolved argv pieces.
func printPlan(w io.Writer, plan *resolvedPlan, actionArgs, targets []string, targetSource string) error {
	if err := writeFormat(w, "home: %s\n", plan.Home); err != nil {
		return err
	}
	if len(plan.WorkspaceRepos) > 0 {
		var names []string
		for _, repo := range plan.WorkspaceRepos {
			names = append(names, repo.Name)
		}
		if err := writeFormat(w, "workspaces: %s\n", strings.Join(names, " ")); err != nil {
			return err
		}
	}
	if len(plan.ContextKeys) > 0 {
		if err := writeFormat(w, "contexts: %s\n", strings.Join(plan.ContextKeys, " ")); err != nil {
			return err
		}
	}
	if err := writeFormat(w, "config: %s\n", strings.Join(plan.ConfigPaths, ", ")); err != nil {
		return err
	}
	if err := writeFormat(w, "env: %s\n", plan.EnvFile); err != nil {
		return err
	}
	if err := writeFormat(w, "stampDir: %s\n", plan.StampDir); err != nil {
		return err
	}
	if len(actionArgs) > 0 {
		if err := writeFormat(w, "actionArgs: %s\n", strings.Join(actionArgs, " ")); err != nil {
			return err
		}
	}
	if err := writeFormat(w, "targetSource: %s\n", targetSource); err != nil {
		return err
	}
	if plan.Makefile != "" {
		if err := writeFormat(w, "makefile: %s\n", plan.Makefile); err != nil {
			return err
		}
	}
	if err := writeLine(w); err != nil {
		return err
	}

	if err := writeLine(w, "tuples:"); err != nil {
		return err
	}
	for _, t := range plan.Tuples {
		if err := writeFormat(w, "  %s\n", t); err != nil {
			return err
		}
	}
	if err := writeLine(w, "targets:"); err != nil {
		return err
	}
	if len(targets) == 0 {
		if err := writeLine(w, "  (none; make will use its default goal)"); err != nil {
			return err
		}
	}
	for _, t := range targets {
		if err := writeFormat(w, "  %s\n", t); err != nil {
			return err
		}
	}
	return nil
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
		if warnErr := writeFormat(stderr, "decomk: log dir %s not writable; falling back to %s (set -log-dir or DECOMK_LOG_DIR to override)\n", plan.LogRoot, fallbackRoot); warnErr != nil {
			return "", warnErr
		}
		return fallbackDir, nil
	}

	return "", fmt.Errorf("create run log dir: tried %s: %v; fallback %s: %v", base, err, fallbackBase, fallbackErr)
}

// applyStartDir changes the current working directory to match -C.
//
// This mirrors the common expectation of tools like make: relative paths provided
// on the command line (for example -config or -makefile paths) are interpreted
// relative to -C.
//
// decomk normalizes -C to an absolute path in os.Args so that nested tooling
// that shells out or re-invokes decomk inherits a stable cwd intent.
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

const (
	// tuplePassThroughValue is the reserved tuple value that requests environment
	// pass-through resolution (for example `BAX=$`).
	tuplePassThroughValue = "$"

	// autoPassThroughPrefix is the env-var namespace that decomk automatically
	// carries into env.sh/make, in addition to resolved config tuples.
	autoPassThroughPrefix = "DECOMK_"
)

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
//
// Intent: Support explicit log-root overrides while keeping the default robust
// in non-root environments via a default-only fallback to <DECOMK_HOME>/log.
// Source: DI-005-20260309-172359 (TODO/005)
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

// warnIfRunRequiresPasswordlessSudo prints a best-effort warning when decomk is
// running in a mode that does not require sudo (plan/dry-run), but the current
// flags/env imply that "decomk run" will attempt to invoke make via passwordless
// sudo.
//
// This is intentionally a warning, not an error: plan output is still useful
// even when a subsequent run would fail.
//
// Intent: Keep "decomk plan" usable without sudo while still surfacing that
// "decomk run" will require passwordless sudo in root-make mode.
// Source: DI-001-20260309-172358 (TODO/001)
func warnIfRunRequiresPasswordlessSudo(makeAsRoot bool, stderr io.Writer) error {
	if !makeAsRoot || os.Geteuid() == 0 {
		return nil
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return writeLine(stderr, "decomk: warning: make will run as root by default but sudo is not in PATH; decomk run will fail unless you set -make-as-root=false")
	}
	cmd := exec.Command("sudo", "-n", "true")
	if err := cmd.Run(); err != nil {
		return writeLine(stderr, "decomk: warning: make will run as root by default but passwordless sudo is not available (sudo -n failed); decomk run will fail unless you set -make-as-root=false")
	}
	return nil
}

// resolveMakeCommand returns the argv-style command prefix to execute make.
//
// If makeAsRoot is true and the current process is not running as root, this
// verifies passwordless sudo is available and returns a "sudo -n ... make"
// prefix.
//
// For dry-run (plan), decomk always returns a plain "make" command so plan does
// not depend on sudo.
//
// Intent: Provide a deterministic privilege boundary for make so Makefile
// recipes can avoid embedding sudo while still supporting user-scoped steps via
// explicit runuser/su calls.
// Source: DI-001-20260309-172358 (TODO/001)
func resolveMakeCommand(makeAsRoot bool, dryRun bool) (command []string, usesSudo bool, err error) {
	if dryRun {
		return []string{"make"}, false, nil
	}
	if !makeAsRoot || os.Geteuid() == 0 {
		return []string{"make"}, false, nil
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return nil, false, fmt.Errorf("running make as root requires sudo in PATH (set -make-as-root=false to disable)")
	}
	cmd := exec.Command("sudo", "-n", "true")
	if err := cmd.Run(); err != nil {
		return nil, false, fmt.Errorf("running make as root requires passwordless sudo (sudo -n failed); set -make-as-root=false to disable: %w", err)
	}

	usesSudo = true

	support := sudoPreserveEnvSupport{
		pathAndGitHubUser: canSudoPreserveEnv("PATH,GITHUB_USER"),
	}
	if !support.pathAndGitHubUser {
		support.pathOnly = canSudoPreserveEnv("PATH")
	}
	return selectSudoMakeCommand(support), usesSudo, nil
}

// sudoPreserveEnvSupport reports which preserve-env configurations sudo accepts
// under the current policy/runtime.
type sudoPreserveEnvSupport struct {
	pathAndGitHubUser bool
	pathOnly          bool
}

// canSudoPreserveEnv reports whether sudo accepts a specific --preserve-env
// variable list for a non-interactive no-op command.
func canSudoPreserveEnv(varList string) bool {
	if strings.TrimSpace(varList) == "" {
		return false
	}
	arg := "--preserve-env=" + varList
	return exec.Command("sudo", "-n", arg, "true").Run() == nil
}

// selectSudoMakeCommand selects the sudo argv prefix for make based on
// preserve-env capability probes.
//
// Intent: Preserve GITHUB_USER (runtime identity) and PATH for root-run make
// when possible, while degrading safely on restrictive sudo builds/policies
// instead of hard failing.
// Source: DI-001-20260422-130000 (TODO/001)
func selectSudoMakeCommand(support sudoPreserveEnvSupport) []string {
	if support.pathAndGitHubUser {
		return []string{"sudo", "-n", "--preserve-env=PATH,GITHUB_USER", "make"}
	}
	if support.pathOnly {
		return []string{"sudo", "-n", "--preserve-env=PATH", "make"}
	}
	return []string{"sudo", "-n", "make"}
}

// chownTreeToCurrentUser ensures path and its contents are owned by the current
// uid/gid.
//
// This is used to keep decomk-managed state (especially stamps) writable by the
// dev user even when make was executed as root via passwordless sudo.
//
// Intent: Prevent root-owned stamp files from breaking future non-root runs.
// Source: DI-001-20260309-172358 (TODO/001)
func chownTreeToCurrentUser(path string) error {
	uid := os.Getuid()
	gid := os.Getgid()
	owner := fmt.Sprintf("%d:%d", uid, gid)
	cmd := exec.Command("sudo", "-n", "chown", "-R", owner, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("sudo -n chown -R %s %s: %w", owner, path, err)
		}
		return fmt.Errorf("sudo -n chown -R %s %s: %w: %s", owner, path, err, msg)
	}
	return nil
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
func resolvePlanFromFlags(f commonFlags) (*resolvedPlan, error) {
	home, err := state.Home(f.home)
	if err != nil {
		return nil, err
	}

	logRoot, logRootExplicit, err := resolveLogRoot(f.logDir)
	if err != nil {
		return nil, err
	}

	workspacesDir := resolveWorkspacesDir(f.workspacesDir)

	// Intent: Keep decomk core deterministic by resolving/running from existing
	// local state only. Stage-0 lifecycle tooling (for example postCreate hooks)
	// is responsible for tool/config clone-pull bootstrap.
	// Source: DI-007-20260311-145221 (TODO/007)
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
	// Intent: Enforce tuple-only config output after macro expansion so target
	// selection happens exclusively through explicit action args.
	// Source: DI-004-20260422-193652 (TODO/004)
	if len(targets) > 0 {
		return nil, fmt.Errorf("invalid config: expanded non-tuple tokens %v; decomk.conf RHS tokens must be tuple assignments (NAME=value) or defined keys", targets)
	}

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
		return nil, nil, fmt.Errorf("no config found; tried %s; set -config/DECOMK_CONFIG or populate %s", strings.Join(tried, ", "), filepath.Join(state.ConfDir(home), "decomk.conf"))
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
	// Intent: Keep decomk.conf tuple-only by requiring every bare RHS token to be
	// a defined key, so config files cannot accidentally smuggle literal targets.
	// Source: DI-004-20260422-193652 (TODO/004)
	if err := contexts.ValidateRefs(defs); err != nil {
		return nil, nil, err
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

// computedVarOrder is the stable order used when emitting decomk-owned computed
// variables on both make's argv and in env export files.
//
// Keeping this list centralized prevents subtle drift between "decomk plan"
// output (env exports), env.sh generation, and the actual make invocation.
//
// Intent: Ensure Makefile recipes and nested scripts can reliably see both the
// dev user identity and the effective make user when privilege-dropping is
// required.
// Source: DI-001-20260309-172358 (TODO/001)
var computedVarOrder = []string{
	"DECOMK_HOME",
	"DECOMK_STAMPDIR",
	"DECOMK_DEV_USER",
	"DECOMK_MAKE_USER",
	"DECOMK_WORKSPACES",
	"DECOMK_CONTEXTS",
	"DECOMK_PACKAGES",
}

// resolveDevUser reports the non-root username that "owns" decomk's state for
// this invocation.
//
// In the common case, decomk itself runs as the dev user and this returns that
// username. When decomk is invoked via sudo, prefer SUDO_USER so Makefile
// recipes have a stable name to drop privileges back to.
//
// Intent: Provide a stable non-root identity for user-scoped recipe steps when
// make is running as root.
// Source: DI-001-20260309-172358 (TODO/001)
func resolveDevUser() string {
	if u := strings.TrimSpace(os.Getenv("SUDO_USER")); u != "" && u != "root" {
		return u
	}

	if cu, err := user.Current(); err == nil {
		u := strings.TrimSpace(cu.Username)
		if u != "" {
			// When a container user has no /etc/passwd entry, user.Current can fall
			// back to returning the numeric uid as "Username". That is not useful as
			// a username (for example for runuser/su), so treat it as unknown.
			if _, err := strconv.Atoi(u); err != nil {
				return u
			}
		}
	}

	for _, key := range []string{"LOGNAME", "USER"} {
		if u := strings.TrimSpace(os.Getenv(key)); u != "" {
			return u
		}
	}
	return ""
}

// resolveMakeUser reports the effective username that make will run as.
//
// When decomk itself is running as root, make runs as root regardless of
// makeAsRoot. When decomk is non-root:
//   - makeAsRoot=true means decomk will invoke make via passwordless sudo -n
//   - makeAsRoot=false means decomk will invoke make as the current user
//
// Intent: Let Makefiles choose when to drop privileges based on a simple,
// explicit signal instead of inferring from environment state.
// Source: DI-001-20260309-172358 (TODO/001)
func resolveMakeUser(makeAsRoot bool, devUser string) string {
	if os.Geteuid() == 0 || makeAsRoot {
		return "root"
	}
	return devUser
}

// computedVars returns decomk-owned computed exports/variables for this plan.
//
// These variables are always defined by decomk and must not be overridden by
// config-provided tuples, because other processes (and Makefile recipes) rely on
// them to describe decomk's actual execution environment.
//
// Intent: Expose computed helpers for Makefiles/scripts so root-run make can
// still perform user-scoped work by explicitly dropping privileges.
// Source: DI-001-20260309-172358 (TODO/001)
func computedVars(plan *resolvedPlan, targets []string, makeAsRoot bool) map[string]string {
	devUser := resolveDevUser()
	var workspaces []string
	for _, repo := range plan.WorkspaceRepos {
		workspaces = append(workspaces, repo.Name)
	}
	return map[string]string{
		"DECOMK_HOME":       plan.Home,
		"DECOMK_STAMPDIR":   plan.StampDir,
		"DECOMK_DEV_USER":   devUser,
		"DECOMK_MAKE_USER":  resolveMakeUser(makeAsRoot, devUser),
		"DECOMK_WORKSPACES": strings.Join(workspaces, " "),
		"DECOMK_CONTEXTS":   strings.Join(plan.ContextKeys, " "),
		"DECOMK_PACKAGES":   strings.Join(targets, " "),
	}
}

// selectTargets determines which make targets decomk should pass on argv.
//
// Callers must pass at least one action arg (enforced in cmdExecute).
// Each arg is interpreted as:
//   - an action variable name (when it matches a resolved tuple variable), or
//   - a literal make target (fallback).
func selectTargets(tuples, actionArgs []string) (targets []string, source string) {
	effective := effectiveTupleValues(tuples)
	return targetsFromActionArgs(actionArgs, effective), "actionArgs"
}

// envMapFromList converts KEY=value strings into a map where the last entry for
// each key wins.
func envMapFromList(envList []string) map[string]string {
	out := make(map[string]string, len(envList))
	for _, kv := range envList {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		out[k] = v
	}
	return out
}

// resolveTuplePassThroughs rewrites `NAME=$` tuples to concrete `NAME=value`
// assignments.
//
// Resolution rules:
//   - if NAME exists in incomingEnv, use that value
//   - else if an earlier tuple already set NAME, reuse that prior value
//   - else fail fast (no implicit empty-string fallback)
//
// Intent: Make env.sh authoritative by resolving passthrough tuples before both
// env export generation and make invocation, so both receive the same final
// values.
// Source: DI-001-20260313-224000 (TODO/001)
func resolveTuplePassThroughs(tuples []string, incomingEnv map[string]string) ([]string, error) {
	out := make([]string, 0, len(tuples))
	effective := make(map[string]string, len(tuples))
	for _, tuple := range tuples {
		name, value, ok := resolve.SplitTuple(tuple)
		if !ok {
			out = append(out, tuple)
			continue
		}
		if value == tuplePassThroughValue {
			if envValue, envOK := incomingEnv[name]; envOK {
				value = envValue
			} else if prior, priorOK := effective[name]; priorOK {
				value = prior
			} else {
				return nil, fmt.Errorf("tuple %s=%s requires %s in environment or a prior tuple fallback", name, tuplePassThroughValue, name)
			}
			tuple = name + "=" + value
		}
		effective[name] = value
		out = append(out, tuple)
	}
	return out, nil
}

// autoPassThroughTuples returns sorted NAME=value tuples for incoming env vars in
// the DECOMK_* namespace.
//
// Only identifier-like keys are emitted so generated entries are valid make
// command-line variable assignments.
//
// Intent: Carry devcontainer-provided DECOMK_* configuration into env.sh and the
// make process tree by default, without requiring explicit tuple duplication in
// decomk.conf.
// Source: DI-001-20260313-224000 (TODO/001)
func autoPassThroughTuples(incomingEnv map[string]string) []string {
	var names []string
	for name := range incomingEnv {
		if !strings.HasPrefix(name, autoPassThroughPrefix) {
			continue
		}
		// Keep only names that are valid NAME=value tuple identifiers.
		if _, _, ok := resolve.SplitTuple(name + "=x"); !ok {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, name+"="+incomingEnv[name])
	}
	return out
}

// canonicalEnvTuples returns the exact tuple sequence decomk treats as the
// authoritative environment contract.
//
// The resulting order is:
//   - incoming DECOMK_* pass-through tuples
//   - resolved config tuples
//   - decomk-computed tuples (last, so they override prior values)
//
// This single sequence feeds both env.sh generation and make invocation to keep
// runtime behavior deterministic.
func canonicalEnvTuples(plan *resolvedPlan, targets []string, makeAsRoot bool, incomingEnv map[string]string) []string {
	out := make([]string, 0, len(incomingEnv)+len(plan.Tuples)+len(computedVarOrder))
	out = append(out, autoPassThroughTuples(incomingEnv)...)
	out = append(out, plan.Tuples...)

	cv := computedVars(plan, targets, makeAsRoot)
	for _, name := range computedVarOrder {
		if v, ok := cv[name]; ok {
			out = append(out, name+"="+v)
		}
	}
	return out
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

// makeInvocation returns the tuple list and process env slice used to invoke
// make.
//
// cookedTuples is the canonical environment contract shared with env.sh export.
func makeInvocation(baseEnv, cookedTuples []string) (tuples []string, env []string) {
	tuples = append([]string(nil), cookedTuples...)
	// Intent: Keep one PATH model by deriving the launcher process env from the
	// same cooked tuple contract that drives env.sh and make argv, even when that
	// means tuple-provided PATH values can affect launcher behavior.
	// Source: DI-001-20260313-174538 (TODO/001)
	env = withEnv(baseEnv, effectiveTupleValues(cookedTuples))
	return tuples, env
}

// writeEnvFile writes the shell-friendly env export file that captures the
// resolved configuration for the run.
//
// This file is intentionally simple: it is designed to be sourced by scripts
// and nested make invocations without requiring eval.
func writeEnvFile(path string, plan *resolvedPlan, cookedTuples []string) error {
	if err := state.EnsureParentDir(path); err != nil {
		return err
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	if err := writeEnvExport(f, plan, cookedTuples); err != nil {
		// Intent: Preserve temp-file close failures alongside export failures so
		// env export write errors are never silently dropped.
		// Source: DI-008-20260412-122157 (TODO/008)
		if closeErr := f.Close(); closeErr != nil {
			return errors.Join(err, fmt.Errorf("close env export temp file after write failure: %w", closeErr))
		}
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
func writeEnvExport(w io.Writer, plan *resolvedPlan, cookedTuples []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := writeFormat(w, "# generated by decomk; do not edit\n"); err != nil {
		return err
	}
	if err := writeFormat(w, "# time: %s\n", now); err != nil {
		return err
	}
	if len(plan.ContextKeys) > 0 {
		if err := writeFormat(w, "# contexts: %s\n", strings.Join(plan.ContextKeys, " ")); err != nil {
			return err
		}
	}
	if len(plan.WorkspaceRepos) > 0 {
		var names []string
		for _, repo := range plan.WorkspaceRepos {
			names = append(names, repo.Name)
		}
		if err := writeFormat(w, "# workspaces: %s\n", strings.Join(names, " ")); err != nil {
			return err
		}
	}
	if err := writeFormat(w, "# config: %s\n", strings.Join(plan.ConfigPaths, ", ")); err != nil {
		return err
	}
	if err := writeLine(w); err != nil {
		return err
	}

	// Intent: Export the same tuple sequence used for make invocation so env.sh is
	// the exact contract for what make and child processes receive.
	// Source: DI-001-20260313-224000 (TODO/001)
	for _, t := range cookedTuples {
		k, v, ok := resolve.SplitTuple(t)
		if !ok {
			continue
		}
		if err := writeExport(w, k, v); err != nil {
			return err
		}
	}
	return nil
}

// writeExport emits one export line using conservative shell quoting.
func writeExport(w io.Writer, name, value string) error {
	return writeFormat(w, "export %s=%s\n", name, shellQuote(value))
}

// shellQuote produces a POSIX-shell-safe single-quoted string.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// Close/open around any embedded single quote.
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
