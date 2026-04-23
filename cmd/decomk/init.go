package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/stevegt/decomk/stage0"
	"github.com/stevegt/decomk/state"
	"github.com/stevegt/envi"
)

// initFlags are the user-facing options for "decomk init".
type initFlags struct {
	repoRoot   string
	name       string
	confURI    string
	toolURI    string
	home       string
	logDir     string
	failNoBoot string
	force      bool
	noPrompt   bool
}

// initWriteResult reports what happened for one stage-0 file.
type initWriteResult struct {
	Path   string
	Status string
}

// cmdInit writes stage-0 files .devcontainer/devcontainer.json and
// .devcontainer/decomk-stage0.sh in the target repo.
//
// Intent: Keep stage-0 bootstrap setup reproducible and easy for new repos by
// generating production-identical lifecycle scaffolding directly from the
// decomk binary.
// Source: DI-001-20260311-161825 (TODO/001)
func cmdInit(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("decomk init", flag.ContinueOnError)
	fs.SetOutput(stderr)

	f := initFlags{
		repoRoot:   "",
		confURI:    envi.String("DECOMK_CONF_URI", ""),
		toolURI:    envi.String("DECOMK_TOOL_URI", stage0.DefaultToolURI),
		home:       envi.String("DECOMK_HOME", state.DefaultHome),
		logDir:     envi.String("DECOMK_LOG_DIR", state.DefaultLogDir),
		failNoBoot: envi.String("DECOMK_FAIL_NOBOOT", stage0.DefaultFailNoBoot),
	}
	addInitFlags(fs, &f)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}

	setFlags := map[string]bool{}
	fs.Visit(func(fl *flag.Flag) {
		setFlags[fl.Name] = true
	})

	repoRootInput := f.repoRoot
	var err error
	if !setFlags["repo-root"] {
		// Intent: Default stage-0 file placement to the current git repo toplevel so
		// running "decomk init" from a nested directory still writes into the
		// repository root by default.
		// Source: DI-001-20260311-164841 (TODO/001)
		repoRootInput, err = gitTopLevelFromDir(".")
		if err != nil {
			return 1, fmt.Errorf("resolve default repo root from current git repo: %w (or set -repo-root)", err)
		}
	}

	repoRoot, err := filepath.Abs(repoRootInput)
	if err != nil {
		return 1, fmt.Errorf("abs repo root %q: %w", repoRootInput, err)
	}
	info, err := os.Stat(repoRoot)
	if err != nil {
		return 1, fmt.Errorf("stat repo root %q: %w", repoRoot, err)
	}
	if !info.IsDir() {
		return 1, fmt.Errorf("repo root is not a directory: %s", repoRoot)
	}

	if !f.force {
		// Intent: Fail fast on existing stage-0 targets before interactive prompts
		// so operators do not answer init questions only to hit overwrite refusal at
		// the end of the command.
		// Source: DI-001-20260423-051500 (TODO/001)
		devcontainerDir := filepath.Join(repoRoot, ".devcontainer")
		jsonPath := filepath.Join(devcontainerDir, "devcontainer.json")
		stage0ScriptPath := filepath.Join(devcontainerDir, "decomk-stage0.sh")
		existing, err := existingInitTargets(jsonPath, stage0ScriptPath)
		if err != nil {
			return 1, err
		}
		if len(existing) > 0 {
			return 1, initExistingTargetsError(existing)
		}
	}

	if f.name == "" {
		f.name = filepath.Base(repoRoot)
	}

	canPrompt := !f.noPrompt && isInteractiveInput(os.Stdin) && isInteractiveInput(os.Stderr)
	if canPrompt {
		if err := promptInitFlags(&f, setFlags, os.Stdin, stderr); err != nil {
			return 1, err
		}
	}

	if f.name == "" {
		return 1, fmt.Errorf("devcontainer name cannot be empty")
	}
	// Intent: Lock stage-0 source configuration onto URI expressions (`go:`/`git:`)
	// so generated init scaffolds use one explicit source contract instead of
	// split mode/package/repo variables.
	// Source: DI-001-20260412-170500 (TODO/001)
	if strings.TrimSpace(f.toolURI) == "" {
		return 1, fmt.Errorf("DECOMK_TOOL_URI template value cannot be empty")
	}
	if !strings.HasPrefix(f.toolURI, "go:") && !strings.HasPrefix(f.toolURI, "git:") {
		return 1, fmt.Errorf("DECOMK_TOOL_URI template value must start with go: or git: (got %q)", f.toolURI)
	}
	if f.confURI != "" && !strings.HasPrefix(f.confURI, "git:") {
		return 1, fmt.Errorf("DECOMK_CONF_URI template value must start with git: when set (got %q)", f.confURI)
	}
	if f.home == "" || !filepath.IsAbs(f.home) {
		return 1, fmt.Errorf("DECOMK_HOME template value must be an absolute path (got %q)", f.home)
	}
	if f.logDir == "" || !filepath.IsAbs(f.logDir) {
		return 1, fmt.Errorf("DECOMK_LOG_DIR template value must be an absolute path (got %q)", f.logDir)
	}
	if err := validateFailNoBootValue(f.failNoBoot); err != nil {
		return 1, err
	}

	// Use the shared stage-0 data model so `decomk init` and generated examples
	// render from the same template contract.
	// Intent: Keep stage-0 inputs consistent across init and stage0gen.
	// Source: DI-001-20260312-141200 (TODO/001)
	data := stage0.DevcontainerTemplateData{
		Name:                 f.name,
		ConfURI:              f.confURI,
		ToolURI:              f.toolURI,
		Home:                 f.home,
		LogDir:               f.logDir,
		FailNoBoot:           f.failNoBoot,
		UpdateContentCommand: stage0.DefaultUpdateContentCommand,
		PostCreateCommand:    stage0.DefaultPostCreateCommand,
	}

	results, err := writeInitStage0(repoRoot, data, f.force)
	if err != nil {
		return 1, err
	}

	if f.confURI == "" {
		if err := writeLine(stderr, "decomk init: warning: DECOMK_CONF_URI is empty; decomk-stage0.sh will fail until conf/decomk.conf exists locally"); err != nil {
			return 1, err
		}
	}
	for _, result := range results {
		if err := writeFormat(stdout, "%s: %s\n", result.Status, result.Path); err != nil {
			return 1, err
		}
	}
	return 0, nil
}

// addInitFlags defines flags for "decomk init".
func addInitFlags(fs *flag.FlagSet, f *initFlags) {
	fs.StringVar(&f.repoRoot, "repo-root", f.repoRoot, "target repo root (writes under <repo-root>/.devcontainer; default: current git repo root)")
	fs.StringVar(&f.name, "name", f.name, "devcontainer name (default: repo basename)")
	fs.StringVar(&f.confURI, "conf-uri", f.confURI, "DECOMK_CONF_URI value for devcontainer.json")
	fs.StringVar(&f.toolURI, "tool-uri", f.toolURI, "DECOMK_TOOL_URI value for devcontainer.json")
	fs.StringVar(&f.home, "home", f.home, "DECOMK_HOME value for devcontainer.json")
	fs.StringVar(&f.logDir, "log-dir", f.logDir, "DECOMK_LOG_DIR value for devcontainer.json")
	fs.StringVar(&f.failNoBoot, "fail-no-boot", f.failNoBoot, "DECOMK_FAIL_NOBOOT value for devcontainer.json (true fails startup on stage-0 errors)")
	fs.BoolVar(&f.force, "force", false, "overwrite existing stage-0 files even when they already exist")
	fs.BoolVar(&f.force, "f", false, "alias for -force")
	fs.BoolVar(&f.noPrompt, "no-prompt", false, "disable interactive prompts for unset values")
}

// promptInitFlags interactively fills unset init values.
func promptInitFlags(f *initFlags, setFlags map[string]bool, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	var err error
	if !setFlags["name"] {
		f.name, err = promptWithDefault(reader, out, "Devcontainer name", f.name)
		if err != nil {
			return err
		}
	}
	if !setFlags["conf-uri"] {
		f.confURI, err = promptWithDefault(reader, out, "DECOMK_CONF_URI", f.confURI)
		if err != nil {
			return err
		}
	}
	if !setFlags["tool-uri"] {
		f.toolURI, err = promptWithDefault(reader, out, "DECOMK_TOOL_URI", f.toolURI)
		if err != nil {
			return err
		}
	}
	if !setFlags["home"] {
		f.home, err = promptWithDefault(reader, out, "DECOMK_HOME", f.home)
		if err != nil {
			return err
		}
	}
	if !setFlags["log-dir"] {
		f.logDir, err = promptWithDefault(reader, out, "DECOMK_LOG_DIR", f.logDir)
		if err != nil {
			return err
		}
	}
	if !setFlags["fail-no-boot"] {
		f.failNoBoot, err = promptWithDefault(reader, out, "DECOMK_FAIL_NOBOOT", f.failNoBoot)
		if err != nil {
			return err
		}
	}
	return nil
}

// promptWithDefault reads one line, returning defaultValue when input is empty.
func promptWithDefault(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		if err := writeFormat(out, "%s [%s]: ", label, defaultValue); err != nil {
			return "", err
		}
	} else {
		if err := writeFormat(out, "%s: ", label); err != nil {
			return "", err
		}
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

// validateFailNoBootValue enforces the accepted stage-0 failure-policy values
// used by DECOMK_FAIL_NOBOOT.
//
// Intent: Keep generated stage-0 config values explicit and valid at init time so
// invalid policy strings fail early instead of surprising users at container boot.
// Source: DI-012-20260423-045339 (TODO/012)
func validateFailNoBootValue(value string) error {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "0", "false", "no", "off", "1", "true", "yes", "on":
		return nil
	default:
		return fmt.Errorf("DECOMK_FAIL_NOBOOT must be one of: true,false,1,0,yes,no,on,off (got %q)", value)
	}
}

// isInteractiveInput reports whether file is connected to a terminal-like input.
func isInteractiveInput(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// gitTopLevelFromDir resolves git's toplevel directory for dir.
func gitTopLevelFromDir(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, msg)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("git rev-parse --show-toplevel returned empty output")
	}
	return root, nil
}

// writeInitStage0 renders embedded templates and writes them to
// <repoRoot>/.devcontainer/.
func writeInitStage0(repoRoot string, data stage0.DevcontainerTemplateData, force bool) ([]initWriteResult, error) {
	devcontainerDir := filepath.Join(repoRoot, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		return nil, err
	}

	// Intent: Use the shared stage-0 renderer so decomk init and generated
	// examples stay in lockstep with one template/data contract.
	// Source: DI-001-20260312-141200 (TODO/001)
	devcontainerJSON, err := stage0.RenderDevcontainerJSON(initDevcontainerJSONTemplate, data)
	if err != nil {
		return nil, err
	}
	stage0Script, err := stage0.RenderStage0Script(initStage0ScriptTemplate)
	if err != nil {
		return nil, err
	}

	jsonPath := filepath.Join(devcontainerDir, "devcontainer.json")
	stage0ScriptPath := filepath.Join(devcontainerDir, "decomk-stage0.sh")

	if !force {
		// Intent: Keep `decomk init` conservative by default: never overwrite or
		// partially scaffold when either stage-0 file already exists, and force users
		// onto an explicit commit/force/difftool reconciliation workflow.
		// Source: DI-001-20260412-194342 (TODO/001)
		existing, err := existingInitTargets(jsonPath, stage0ScriptPath)
		if err != nil {
			return nil, err
		}
		if len(existing) > 0 {
			return nil, initExistingTargetsError(existing)
		}
	}

	var results []initWriteResult
	jsonStatus, err := writeInitFile(jsonPath, devcontainerJSON, 0o644, force)
	if err != nil {
		return nil, err
	}
	results = append(results, initWriteResult{Path: jsonPath, Status: jsonStatus})

	scriptStatus, err := writeInitFile(stage0ScriptPath, stage0Script, 0o755, force)
	if err != nil {
		return nil, err
	}
	results = append(results, initWriteResult{Path: stage0ScriptPath, Status: scriptStatus})

	return results, nil
}

// writeInitFile writes stage-0 content with a simple overwrite policy.
//
// Status values:
//   - "created": file did not exist
//   - "updated": file existed and content changed (force mode)
//   - "unchanged": file existed with identical content (force mode)
func writeInitFile(path string, content []byte, mode os.FileMode, force bool) (status string, err error) {
	_, statErr := os.Stat(path)
	existed := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}

	if existed {
		existing, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if bytes.Equal(existing, content) {
			if err := stage0.EnsureMode(path, mode); err != nil {
				return "", err
			}
			return "unchanged", nil
		}
		if !force {
			return "", fmt.Errorf("file exists with different content: %s (use -f/-force to overwrite)", path)
		}
	}

	// Intent: Write stage-0 files atomically (temp file + rename) so
	// interruptions cannot leave a partially-written devcontainer config/script.
	// Source: DI-001-20260311-175002 (TODO/001)
	if err := stage0.WriteFileAtomic(path, content, mode); err != nil {
		return "", err
	}

	if existed {
		return "updated", nil
	}
	return "created", nil
}

// existingInitTargets reports which init targets already exist.
func existingInitTargets(paths ...string) ([]string, error) {
	existing := make([]string, 0, len(paths))
	for _, path := range paths {
		_, err := os.Stat(path)
		if err == nil {
			existing = append(existing, path)
			continue
		}
		if os.IsNotExist(err) {
			continue
		}
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	return existing, nil
}

// initExistingTargetsError formats the strict non-force guidance for existing
// stage-0 files.
func initExistingTargetsError(existing []string) error {
	var builder strings.Builder
	builder.WriteString("refusing to overwrite existing stage-0 file(s) without -f/-force:\n")
	for _, path := range existing {
		builder.WriteString("  - ")
		builder.WriteString(path)
		builder.WriteString("\n")
	}
	builder.WriteString("recommended workflow:\n")
	builder.WriteString("  1) git commit the current .devcontainer files\n")
	builder.WriteString("  2) run decomk init -f ...\n")
	builder.WriteString("  3) resolve with: git difftool -- .devcontainer/devcontainer.json .devcontainer/decomk-stage0.sh")
	return errors.New(builder.String())
}
