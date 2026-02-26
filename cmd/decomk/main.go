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
	toolRepo    string
	confRepo    string
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
	fs.StringVar(&f.toolRepo, "tool-repo", "", "decomk tool repo URL to clone/pull into <home>/decomk (also DECOMK_TOOL_REPO)")
	fs.StringVar(&f.confRepo, "conf-repo", "", "config repo URL to clone/pull into <home>/conf (also DECOMK_CONF_REPO)")
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

	// StampDir is decomk's global make working directory (the stamps directory).
	//
	// decomk uses a single stamp directory for the whole container because it is
	// intended to configure the container (tools, caches, etc.), not to manage
	// per-repo build artifacts.
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

	plans, err := resolvePlansFromFlags(f, stderr)
	if err != nil {
		return 1, err
	}

	for i, plan := range plans {
		if err := writeEnvSnapshot(plan.EnvFile, plan); err != nil {
			return 1, err
		}

		if i > 0 {
			fmt.Fprintln(stdout)
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

	plans, err := resolvePlansFromFlags(f, stderr)
	if err != nil {
		return 1, err
	}

	if len(plans) == 0 {
		return 1, fmt.Errorf("no workspaces found to apply")
	}
	for _, plan := range plans {
		if plan.Makefile == "" {
			return 1, fmt.Errorf("no Makefile found for workspace %s; use -makefile to set an explicit path", plan.WorkspaceRoot)
		}
	}

	// Prevent concurrent stamp mutation for the container.
	lock, err := state.LockFile(state.StampsLockPath(plans[0].Home))
	if err != nil {
		return 1, fmt.Errorf("lock stamps: %w", err)
	}
	defer lock.Close()

	// Ensure the global stamp directory exists and normalize mtime semantics once
	// per invocation (not per workspace).
	if err := state.EnsureDir(plans[0].StampDir); err != nil {
		return 1, err
	}
	if err := state.TouchExistingStamps(plans[0].StampDir, time.Now()); err != nil {
		return 1, fmt.Errorf("touch stamps: %w", err)
	}

	for i, plan := range plans {
		if err := writeEnvSnapshot(plan.EnvFile, plan); err != nil {
			return 1, err
		}

		// Include sub-second resolution and pid to avoid collisions when two runs start
		// close together (otherwise one run can clobber the other's audit log).
		runID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + strconv.Itoa(os.Getpid()) + "-" + strconv.Itoa(i)
		auditDir, err := createAuditDir(plan.Home, runID)
		if err != nil {
			return 1, err
		}
		logPath := filepath.Join(auditDir, "make.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			return 1, err
		}

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
		_ = logFile.Close()
		if runErr != nil {
			// Preserve make's exit status.
			return exitCode, fmt.Errorf("make failed for workspace %s context %s (exit %d); log: %s: %w", plan.WorkspaceRoot, plan.ContextKey, exitCode, logPath, runErr)
		}
	}
	return 0, nil
}

// createAuditDir creates a per-run audit directory and returns its path.
//
// The directory must be unique for each invocation so audit logs never
// overwrite each other.
func createAuditDir(home, runID string) (string, error) {
	base := state.AuditDir(home, runID)
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

// resolvePlansFromFlags builds one or more fully-resolved plans from the user-
// facing flags.
//
// In the common devcontainer case, there may be multiple git repos under the
// workspace parent directory (often /workspaces). decomk scans sibling repos and
// resolves a context for each so the container can be configured based on all
// WIP repos present.
//
// If the user explicitly sets a context (via -context or DECOMK_CONTEXT), decomk
// resolves a single plan for the primary workspace only. This makes debugging
// and experimentation predictable.
func resolvePlansFromFlags(f commonFlags, stderr io.Writer) ([]*resolvedPlan, error) {
	home, err := state.Home(f.home)
	if err != nil {
		return nil, err
	}

	primaryWorkspaceRoot, err := state.WorkspaceRoot(f.workspace)
	if err != nil {
		return nil, err
	}

	// Before doing any other work, update decomk itself (isconf-style). This
	// may rebuild and re-exec into the updated binary under <home>/decomk.
	if err := selfUpdateTool(home, primaryWorkspaceRoot, f.toolRepo, f.verbose, stderr); err != nil {
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

	// If the user explicitly sets a context, do not scan sibling workspaces.
	explicitContext := f.context
	if explicitContext == "" {
		explicitContext = os.Getenv("DECOMK_CONTEXT")
	}
	if explicitContext != "" {
		repo := inspectWorkspaceRepo(primaryWorkspaceRoot)
		plan, err := resolvePlanForRepo(home, repo, explicitConfig, explicitContext, f)
		if err != nil {
			return nil, err
		}
		return []*resolvedPlan{plan}, nil
	}

	// Otherwise scan sibling repos under the workspace parent directory.
	repos, err := discoverWorkspaceRepos(primaryWorkspaceRoot)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		repos = []workspaceRepo{inspectWorkspaceRepo(primaryWorkspaceRoot)}
	}

	plans := make([]*resolvedPlan, 0, len(repos))
	for _, repo := range repos {
		plan, err := resolvePlanForRepo(home, repo, explicitConfig, "", f)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
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

// discoverWorkspaceRepos finds git repos under the parent directory of the
// provided workspace root.
//
// This intentionally does not recurse: the expected layout is one git checkout
// per child directory under a common parent (e.g., /workspaces/<repo>).
func discoverWorkspaceRepos(primaryWorkspaceRoot string) ([]workspaceRepo, error) {
	// If the primary workspace is not a git repo, we don't have a safe way to
	// infer a sibling root. Fall back to a single-workspace plan.
	if !isGitRepoRoot(primaryWorkspaceRoot) {
		return nil, nil
	}

	parent := filepath.Dir(primaryWorkspaceRoot)
	entries, err := os.ReadDir(parent)
	if err != nil {
		// If we can't scan siblings, fall back to applying only the current repo.
		return nil, nil
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
		root := filepath.Join(parent, name)
		if !isGitRepoRoot(root) {
			continue
		}
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

// resolvePlanForRepo resolves decomk config + context into a concrete plan for
// a single workspace repo.
func resolvePlanForRepo(home string, repo workspaceRepo, explicitConfig, explicitContext string, f commonFlags) (*resolvedPlan, error) {
	workspaceRoot := repo.Root

	workspaceKey, err := state.WorkspaceKey(workspaceRoot, repo.OwnerRepo)
	if err != nil {
		return nil, err
	}

	defs, configPaths, err := loadDefs(home, workspaceRoot, explicitConfig)
	if err != nil {
		return nil, err
	}

	var contextKey string
	if explicitContext != "" {
		contextKey, err = selectContextKey(defs, explicitContext)
	} else {
		contextKey, err = selectContextKeyForRepo(defs, repo)
	}
	if err != nil {
		return nil, err
	}

	seed := seedTokens(defs, contextKey)
	expanded, err := expand.ExpandTokens(expand.Defs(defs), seed, expand.Options{MaxDepth: f.maxExpDepth})
	if err != nil {
		return nil, err
	}
	tuples, targets := resolve.Partition(expanded)

	stampDir := state.StampDir(home)
	envFile := state.EnvFile(home, contextKey)

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
func selfUpdateTool(home, primaryWorkspaceRoot, repoURL string, verbose bool, stderr io.Writer) error {
	if repoURL == "" {
		repoURL = os.Getenv("DECOMK_TOOL_REPO")
	}

	// Serialize clone/pull/build operations so concurrent decomk invocations can't
	// corrupt the tool working tree or clobber the built binary.
	lock, err := state.LockFile(state.ToolLockPath(home))
	if err != nil {
		return fmt.Errorf("lock tool repo: %w", err)
	}

	changed, err := ensureToolRepo(home, primaryWorkspaceRoot, repoURL, verbose, stderr)
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
// workspace checkout (e.g., "<workspaceParent>/decomk"), falling back to
// defaultToolRepoURL.
//
// It returns changed=true if the clone was newly created or if git pull changed
// HEAD.
func ensureToolRepo(home, primaryWorkspaceRoot, repoURL string, verbose bool, stderr io.Writer) (changed bool, err error) {
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
			cloneURL = inferToolRepoURL(primaryWorkspaceRoot)
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
// include a WIP clone at "<workspaceParent>/decomk". If it exists, we prefer its
// origin URL, falling back to the local path itself (which works as a git clone
// source).
func inferToolRepoURL(primaryWorkspaceRoot string) string {
	// If we can't determine a stable workspace root, we can't infer siblings.
	if primaryWorkspaceRoot == "" {
		return ""
	}
	parent := filepath.Dir(primaryWorkspaceRoot)
	candidate := filepath.Join(parent, "decomk")
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

	if configRepo, ok := configRepoConfigPath(home); ok {
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
		tried := append([]string(nil), configRepoConfigCandidates(home)...)
		tried = append(tried, repoLocal)
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
func configRepoConfigCandidates(home string) []string {
	return []string{
		filepath.Join(state.ConfDir(home), "etc", "decomk.conf"),
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

// selectContextKeyForRepo chooses a context key for a specific workspace repo.
//
// Candidate order is "most specific to least specific":
//  1. owner/repo (derived from remote origin URL when available)
//  2. repo name (derived from origin URL or directory basename)
//  3. workspace directory basename
//  4. DEFAULT
//
// The first candidate that exists as a key in defs wins.
func selectContextKeyForRepo(defs contexts.Defs, repo workspaceRepo) (string, error) {
	seen := make(map[string]bool, 4)
	var candidates []string

	add := func(s string) {
		if s == "" {
			return
		}
		if seen[s] {
			return
		}
		seen[s] = true
		candidates = append(candidates, s)
	}

	add(repo.OwnerRepo)
	add(repo.RepoName)
	add(repo.Name)
	add("DEFAULT")

	for _, c := range candidates {
		if _, ok := defs[c]; ok {
			return c, nil
		}
	}
	return "", fmt.Errorf("no matching context found for workspace %s; tried %v", repo.Root, candidates)
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
// repo" (under <DECOMK_HOME>/conf). Repo-local decomk.conf is intended as an
// overlay, not as the canonical source of make recipes.
//
// Selection order (first match wins):
//  1. sibling of explicitConfig (if non-empty)
//  2. <DECOMK_HOME>/conf/etc/Makefile (or <DECOMK_HOME>/conf/Makefile)
//  3. workspaceRoot/Makefile (workspace-local fallback)
func findDefaultMakefile(home, workspaceRoot, explicitConfig string) string {
	if explicitConfig != "" {
		candidate := filepath.Join(filepath.Dir(explicitConfig), "Makefile")
		if fileExists(candidate) {
			return candidate
		}
	}
	for _, candidate := range []string{
		filepath.Join(state.ConfDir(home), "etc", "Makefile"),
		filepath.Join(state.ConfDir(home), "Makefile"),
	} {
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
