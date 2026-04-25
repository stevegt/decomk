package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/stevegt/decomk/confrepo"
	"github.com/stevegt/decomk/stage0"
	"github.com/stevegt/decomk/state"
	"github.com/stevegt/envi"
)

// initFlags are the user-facing options for "decomk init".
type initFlags struct {
	confMode bool
	repoRoot string
	name     string
	image    string
	// buildDockerfile/buildContext are sourced from an existing devcontainer.json
	// when present so `decomk init -f` can preserve build-based configs while still
	// using one rendering path.
	buildDockerfile    string
	buildContext       string
	confURI            string
	toolURI            string
	home               string
	logDir             string
	remoteIdentityUser string
	remoteIdentityUID  string
	failNoBoot         string
	force              bool
	noPrompt           bool
}

type initProducerDefaults struct {
	ToolURI string
	Home    string
	LogDir  string
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
		confMode:           false,
		repoRoot:           "",
		confURI:            envi.String("DECOMK_CONF_URI", ""),
		toolURI:            envi.String("DECOMK_TOOL_URI", stage0.DefaultToolURI),
		image:              stage0.DefaultDevcontainerImage,
		home:               envi.String("DECOMK_HOME", state.DefaultHome),
		logDir:             envi.String("DECOMK_LOG_DIR", state.DefaultLogDir),
		remoteIdentityUser: stage0.DefaultDevcontainerUser,
		remoteIdentityUID:  stage0.DefaultDevcontainerUID,
		// Intent: Keep init fail-policy defaults local-first then builtin-false,
		// independent from producer/env imports, so stage-0 boot policy remains an
		// explicit repo decision.
		// Source: DI-001-20260425-113454 (TODO/001)
		failNoBoot: stage0.DefaultFailNoBoot,
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

	if f.confMode {
		// Intent: Keep producer and consumer initialization under one command
		// surface (`decomk init`) while preserving producer-specific scaffold
		// behavior behind an explicit `-conf` mode switch.
		// Source: DI-001-20260424-190437 (TODO/001)
		return runInitConfMode(&f, setFlags, repoRoot, stdout, stderr)
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

	// Intent: Reuse existing stage-0 values as interactive/no-prompt defaults when
	// `decomk init -f` is used, so reruns do not force users to re-enter config by
	// hand.
	// Source: DI-001-20260423-140628 (TODO/001)
	existingDefaults, hasExistingDefaults, err := loadInitExistingDevcontainerDefaults(repoRoot)
	if err != nil {
		if writeErr := writeFormat(stderr, "decomk init: warning: unable to parse existing .devcontainer/devcontainer.json for defaults: %v\n", err); writeErr != nil {
			return 1, writeErr
		}
	} else if hasExistingDefaults {
		applyInitDefaultsFromExistingDevcontainer(&f, setFlags, existingDefaults)
	}

	canPrompt := !f.noPrompt && isInteractiveInput(os.Stdin) && isInteractiveInput(os.Stderr)
	if canPrompt && !setFlags["conf-uri"] {
		// Intent: Prompt for DECOMK_CONF_URI before producer-default import so
		// interactive init can replace placeholder defaults and still inherit
		// producer tool/home/log defaults in the same run.
		// Source: DI-001-20260425-113454 (TODO/001)
		reader := bufio.NewReader(os.Stdin)
		confURIValue, promptErr := promptWithDefault(reader, stderr, "DECOMK_CONF_URI", f.confURI)
		if promptErr != nil {
			return 1, promptErr
		}
		f.confURI = confURIValue
		setFlags["conf-uri"] = true
	}

	if strings.TrimSpace(f.confURI) != "" {
		// Intent: Keep consumer init conf-driven for shared bootstrap defaults
		// (`DECOMK_TOOL_URI`, `DECOMK_HOME`, `DECOMK_LOG_DIR`) without using the
		// producer repo as an identity transport layer.
		// Source: DI-001-20260425-113454 (TODO/001)
		producerDefaults, err := loadProducerDefaultsFromConfURI(f.confURI)
		if err != nil {
			return 1, fmt.Errorf("resolve init defaults from producer conf repo %q: %w", f.confURI, err)
		}
		applyInitDefaultsFromProducerConf(&f, setFlags, producerDefaults)
	}

	if f.name == "" {
		f.name = filepath.Base(repoRoot)
	}

	if canPrompt {
		if err := promptInitFlags(&f, setFlags, os.Stdin, stderr); err != nil {
			return 1, err
		}
	}

	if f.name == "" {
		return 1, fmt.Errorf("devcontainer name cannot be empty")
	}
	if strings.TrimSpace(f.buildDockerfile) == "" && strings.TrimSpace(f.image) == "" {
		return 1, fmt.Errorf("devcontainer image template value cannot be empty when no build.dockerfile is configured")
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
	if strings.TrimSpace(f.confURI) == "" {
		return 1, fmt.Errorf("DECOMK_CONF_URI template value cannot be empty in consumer mode (set -conf-uri/DECOMK_CONF_URI)")
	}
	if !strings.HasPrefix(f.confURI, "git:") {
		return 1, fmt.Errorf("DECOMK_CONF_URI template value must start with git: (got %q)", f.confURI)
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
		Name:                     f.name,
		BuildDockerfile:          f.buildDockerfile,
		BuildContext:             f.buildContext,
		Image:                    f.image,
		DisableRemoteIdentityEnv: true,
		UpdateRemoteUserUID:      boolPointer(false),
		ConfURI:                  f.confURI,
		ToolURI:                  f.toolURI,
		Home:                     f.home,
		LogDir:                   f.logDir,
		FailNoBoot:               f.failNoBoot,
		UpdateContentCommand:     stage0.DefaultUpdateContentCommand,
		PostCreateCommand:        stage0.DefaultPostCreateCommand,
	}

	results, err := writeInitStage0(repoRoot, data, f.force)
	if err != nil {
		return 1, err
	}
	for _, result := range results {
		if err := writeFormat(stdout, "%s: %s\n", result.Status, result.Path); err != nil {
			return 1, err
		}
	}
	return 0, nil
}

// runInitConfMode executes `decomk init -conf` producer scaffolding behavior.
//
// Intent: Keep producer `.devcontainer` identity and URI defaults managed by the
// same interactive/non-interactive flow as consumer init, while writing the full
// shared-conf starter tree.
// Source: DI-001-20260424-190437 (TODO/001)
func runInitConfMode(f *initFlags, setFlags map[string]bool, repoRoot string, stdout, stderr io.Writer) (int, error) {
	if !setFlags["conf-uri"] && strings.TrimSpace(f.confURI) == "" {
		f.confURI = confrepo.DefaultConfURI
	}
	if !f.force {
		paths := make([]string, 0, len(confrepo.ManagedPaths()))
		for _, relPath := range confrepo.ManagedPaths() {
			paths = append(paths, filepath.Join(repoRoot, relPath))
		}
		existing, err := existingInitTargets(paths...)
		if err != nil {
			return 1, err
		}
		if len(existing) > 0 {
			return 1, initConfExistingTargetsError(existing)
		}
	}

	// Intent: Keep `decomk init -conf -f` reruns ergonomic by reusing existing
	// producer devcontainer values as defaults when present.
	// Source: DI-001-20260424-190437 (TODO/001)
	existingDefaults, hasExistingDefaults, err := loadInitExistingDevcontainerDefaults(repoRoot)
	if err != nil {
		if writeErr := writeFormat(stderr, "decomk init -conf: warning: unable to parse existing .devcontainer/devcontainer.json for defaults: %v\n", err); writeErr != nil {
			return 1, writeErr
		}
	} else if hasExistingDefaults {
		applyInitDefaultsFromExistingDevcontainer(f, setFlags, existingDefaults)
	}
	dockerfileDefaults, hasDockerfileDefaults, err := loadInitExistingDockerfileDefaults(repoRoot)
	if err != nil {
		if writeErr := writeFormat(stderr, "decomk init -conf: warning: unable to parse existing .devcontainer/Dockerfile for defaults: %v\n", err); writeErr != nil {
			return 1, writeErr
		}
	} else if hasDockerfileDefaults {
		applyInitDefaultsFromExistingDockerfile(f, setFlags, dockerfileDefaults)
	}

	if f.name == "" {
		// Intent: Keep producer `init -conf` naming defaults identical to consumer
		// `init` so users always get repo-basename defaults unless they pass
		// `-name` or reuse an existing value from a prior scaffold.
		// Source: DI-001-20260424-193612 (TODO/001)
		f.name = filepath.Base(repoRoot)
	}
	// Producer mode always emits a build-backed starter profile.
	f.buildDockerfile = "Dockerfile"
	f.buildContext = ".."

	canPrompt := !f.noPrompt && isInteractiveInput(os.Stdin) && isInteractiveInput(os.Stderr)
	if canPrompt {
		if err := promptInitFlags(f, setFlags, os.Stdin, stderr); err != nil {
			return 1, err
		}
	}

	if f.name == "" {
		return 1, fmt.Errorf("devcontainer name cannot be empty")
	}
	if strings.TrimSpace(f.image) == "" {
		return 1, fmt.Errorf("producer Dockerfile base image cannot be empty (set -image)")
	}
	if strings.TrimSpace(f.confURI) == "" {
		return 1, fmt.Errorf("DECOMK_CONF_URI template value cannot be empty")
	}
	if !strings.HasPrefix(f.confURI, "git:") {
		return 1, fmt.Errorf("DECOMK_CONF_URI template value must start with git: (got %q)", f.confURI)
	}
	if strings.TrimSpace(f.toolURI) == "" {
		return 1, fmt.Errorf("DECOMK_TOOL_URI template value cannot be empty")
	}
	if !strings.HasPrefix(f.toolURI, "go:") && !strings.HasPrefix(f.toolURI, "git:") {
		return 1, fmt.Errorf("DECOMK_TOOL_URI template value must start with go: or git: (got %q)", f.toolURI)
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
	if err := validateRemoteIdentity(f.remoteIdentityUser, f.remoteIdentityUID); err != nil {
		return 1, err
	}

	data := confrepo.ProducerDevcontainerDataWithIdentity(f.name, f.remoteIdentityUser, f.remoteIdentityUID)
	data.Image = f.image
	data.ConfURI = f.confURI
	data.ToolURI = f.toolURI
	data.Home = f.home
	data.LogDir = f.logDir
	data.FailNoBoot = f.failNoBoot

	results, err := writeInitConfScaffold(repoRoot, data, f.force)
	if err != nil {
		return 1, err
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
	fs.BoolVar(&f.confMode, "conf", f.confMode, "producer mode: scaffold a shared conf repo starter tree at repo root")
	fs.StringVar(&f.repoRoot, "repo-root", f.repoRoot, "target repo root (writes under <repo-root>/.devcontainer; default: current git repo root)")
	fs.StringVar(&f.name, "name", f.name, "devcontainer name (default: repo basename)")
	fs.StringVar(&f.image, "image", f.image, "consumer mode: devcontainer image value when no build dockerfile is configured; producer mode (-conf): Dockerfile FROM base image")
	fs.StringVar(&f.confURI, "conf-uri", f.confURI, "DECOMK_CONF_URI value for devcontainer.json")
	fs.StringVar(&f.toolURI, "tool-uri", f.toolURI, "DECOMK_TOOL_URI value for devcontainer.json")
	fs.StringVar(&f.home, "home", f.home, "DECOMK_HOME value for devcontainer.json")
	fs.StringVar(&f.logDir, "log-dir", f.logDir, "DECOMK_LOG_DIR value for devcontainer.json")
	fs.StringVar(&f.remoteIdentityUser, "remote-user", f.remoteIdentityUser, "DECOMK_REMOTE_USER value for generated producer Dockerfile ENV (producer mode)")
	fs.StringVar(&f.remoteIdentityUID, "remote-uid", f.remoteIdentityUID, "DECOMK_REMOTE_UID value for generated producer Dockerfile ENV (producer mode)")
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
	if !setFlags["image"] && (strings.TrimSpace(f.buildDockerfile) == "" || f.confMode) {
		imagePromptLabel := "Image"
		if f.confMode {
			imagePromptLabel = "Base image (Dockerfile FROM)"
		}
		f.image, err = promptWithDefault(reader, out, imagePromptLabel, f.image)
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
	if f.confMode && !setFlags["remote-user"] {
		f.remoteIdentityUser, err = promptWithDefault(reader, out, "DECOMK_REMOTE_USER", f.remoteIdentityUser)
		if err != nil {
			return err
		}
	}
	if f.confMode && !setFlags["remote-uid"] {
		f.remoteIdentityUID, err = promptWithDefault(reader, out, "DECOMK_REMOTE_UID", f.remoteIdentityUID)
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

// validateRemoteIdentity enforces producer stage-0 identity values rendered into
// producer Dockerfile ENV declarations.
//
// Intent: Keep init-time producer identity validation explicit so invalid
// DECOMK_REMOTE_USER/DECOMK_REMOTE_UID values fail during scaffolding instead
// of producing stage-0 runtime failures in downstream consumer images.
// Source: DI-001-20260425-113454 (TODO/001)
func validateRemoteIdentity(remoteIdentityUser, remoteIdentityUID string) error {
	if strings.TrimSpace(remoteIdentityUser) == "" {
		return fmt.Errorf("DECOMK_REMOTE_USER template value cannot be empty")
	}
	if strings.ContainsAny(remoteIdentityUser, " \t\r\n") {
		return fmt.Errorf("DECOMK_REMOTE_USER must not contain whitespace (got %q)", remoteIdentityUser)
	}
	uidValue, err := strconv.Atoi(strings.TrimSpace(remoteIdentityUID))
	if err != nil {
		return fmt.Errorf("DECOMK_REMOTE_UID must be an integer (got %q)", remoteIdentityUID)
	}
	if uidValue <= 0 {
		return fmt.Errorf("DECOMK_REMOTE_UID must be positive (got %d)", uidValue)
	}
	return nil
}

func loadProducerDefaultsFromConfURI(confURI string) (initProducerDefaults, error) {
	repoURL, gitRef, err := parseGitSourceURI(confURI)
	if err != nil {
		return initProducerDefaults{}, err
	}
	tmpRoot, err := os.MkdirTemp("/tmp", "decomk-init-conf-*")
	if err != nil {
		return initProducerDefaults{}, fmt.Errorf("create temp clone root: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpRoot); removeErr != nil {
			// Intent: Cleanup of temporary conf clones is best-effort and
			// non-fatal for init; clone/read errors are still returned explicitly.
			// Source: DI-001-20260424-190437 (TODO/001)
		}
	}()

	repoDir := filepath.Join(tmpRoot, "confrepo")
	if err := runGitCommand("", "clone", repoURL, repoDir); err != nil {
		return initProducerDefaults{}, err
	}
	if err := checkoutGitRef(repoDir, gitRef); err != nil {
		return initProducerDefaults{}, err
	}

	defaults, hasDefaults, err := loadInitExistingDevcontainerDefaults(repoDir)
	if err != nil {
		return initProducerDefaults{}, err
	}
	if !hasDefaults {
		return initProducerDefaults{}, fmt.Errorf("producer conf repo does not contain .devcontainer/devcontainer.json")
	}

	return initProducerDefaults{
		ToolURI: strings.TrimSpace(defaults.ToolURI),
		Home:    strings.TrimSpace(defaults.Home),
		LogDir:  strings.TrimSpace(defaults.LogDir),
	}, nil
}

func parseGitSourceURI(gitURI string) (repoURL string, gitRef string, err error) {
	if !strings.HasPrefix(gitURI, "git:") {
		return "", "", fmt.Errorf("git source URI must start with git: (got %q)", gitURI)
	}
	payload := strings.TrimPrefix(gitURI, "git:")
	if payload == "" {
		return "", "", fmt.Errorf("git source URI is missing repository URL: %q", gitURI)
	}

	repoURL = payload
	query := ""
	if questionMarkIndex := strings.Index(payload, "?"); questionMarkIndex >= 0 {
		repoURL = payload[:questionMarkIndex]
		query = payload[questionMarkIndex+1:]
	}
	if strings.TrimSpace(repoURL) == "" {
		return "", "", fmt.Errorf("git source URI is missing repository URL: %q", gitURI)
	}
	if query != "" {
		values, parseErr := url.ParseQuery(query)
		if parseErr != nil {
			return "", "", fmt.Errorf("parse git URI query %q: %w", query, parseErr)
		}
		gitRef = strings.TrimSpace(values.Get("ref"))
	}
	return repoURL, gitRef, nil
}

func checkoutGitRef(repoDir, gitRef string) error {
	if strings.TrimSpace(gitRef) == "" {
		return nil
	}
	if err := runGitCommand(repoDir, "checkout", "--detach", gitRef); err == nil {
		return nil
	}
	if err := runGitCommand(repoDir, "checkout", "-B", gitRef, "origin/"+gitRef); err == nil {
		return nil
	}
	if err := runGitCommand(repoDir, "fetch", "--prune", "origin", gitRef); err != nil {
		return err
	}
	return runGitCommand(repoDir, "checkout", "--detach", "FETCH_HEAD")
}

func runGitCommand(dir string, args ...string) error {
	command := exec.Command("git", args...)
	if dir != "" {
		command.Dir = dir
	}
	output, err := command.CombinedOutput()
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput == "" {
			return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, trimmedOutput)
	}
	return nil
}

// applyInitDefaultsFromProducerConf applies producer-conf defaults only when
// corresponding CLI flags are unset.
//
// Intent: Keep consumer init precedence deterministic (`CLI > producer >
// existing local > built-ins`) for shared bootstrap defaults while excluding
// identity transport from producer devcontainer metadata.
// Source: DI-001-20260425-113454 (TODO/001)
func applyInitDefaultsFromProducerConf(f *initFlags, setFlags map[string]bool, defaults initProducerDefaults) {
	if !setFlags["tool-uri"] && defaults.ToolURI != "" {
		f.toolURI = defaults.ToolURI
	}
	if !setFlags["home"] && defaults.Home != "" {
		f.home = defaults.Home
	}
	if !setFlags["log-dir"] && defaults.LogDir != "" {
		f.logDir = defaults.LogDir
	}
}

func boolPointer(value bool) *bool {
	return &value
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

// initExistingDevcontainerDefaults captures stage-0 template values recovered
// from an existing .devcontainer/devcontainer.json.
type initExistingDevcontainerDefaults struct {
	Name                string
	Image               string
	BuildDockerfile     string
	BuildContext        string
	RemoteUser          string
	ContainerUser       string
	UpdateRemoteUserUID *bool
	ConfURI             string
	ToolURI             string
	Home                string
	LogDir              string
	RemoteIdentityUser  string
	RemoteIdentityUID   string
	FailNoBoot          string
}

// initExistingDockerfileDefaults captures producer defaults recovered from an
// existing .devcontainer/Dockerfile.
type initExistingDockerfileDefaults struct {
	BaseImage          string
	RemoteIdentityUser string
	RemoteIdentityUID  string
}

// applyInitDefaultsFromExistingDevcontainer merges existing devcontainer values
// into init flags when the corresponding CLI flags were not explicitly set.
func applyInitDefaultsFromExistingDevcontainer(f *initFlags, setFlags map[string]bool, defaults initExistingDevcontainerDefaults) {
	if !setFlags["name"] && defaults.Name != "" {
		f.name = defaults.Name
	}
	if !setFlags["image"] && defaults.Image != "" {
		f.image = defaults.Image
	}
	if defaults.BuildDockerfile != "" {
		f.buildDockerfile = defaults.BuildDockerfile
		f.buildContext = defaults.BuildContext
		// Build-backed devcontainers should continue to render a build stanza
		// unless the operator explicitly passes -image to switch modes. Producer
		// mode keeps image defaults because `-image` supplies Dockerfile FROM.
		if !setFlags["image"] {
			if !f.confMode {
				f.image = ""
			}
		}
	}
	if !setFlags["conf-uri"] && defaults.ConfURI != "" {
		f.confURI = defaults.ConfURI
	}
	if !setFlags["tool-uri"] && defaults.ToolURI != "" {
		f.toolURI = defaults.ToolURI
	}
	if !setFlags["home"] && defaults.Home != "" {
		f.home = defaults.Home
	}
	if !setFlags["log-dir"] && defaults.LogDir != "" {
		f.logDir = defaults.LogDir
	}
	if !setFlags["remote-user"] {
		for _, candidate := range []string{defaults.RemoteIdentityUser, defaults.RemoteUser, defaults.ContainerUser} {
			if candidate != "" {
				f.remoteIdentityUser = candidate
				break
			}
		}
	}
	if !setFlags["remote-uid"] && defaults.RemoteIdentityUID != "" {
		f.remoteIdentityUID = defaults.RemoteIdentityUID
	}
	if !setFlags["fail-no-boot"] && defaults.FailNoBoot != "" {
		f.failNoBoot = defaults.FailNoBoot
	}
}

// applyInitDefaultsFromExistingDockerfile merges producer Dockerfile defaults
// into init flags when corresponding CLI flags are not explicitly set.
//
// Intent: Keep `decomk init -conf -f` reruns ergonomic by reusing existing
// Dockerfile identity/base-image values as prompt defaults.
// Source: DI-001-20260425-232447 (TODO/001)
func applyInitDefaultsFromExistingDockerfile(f *initFlags, setFlags map[string]bool, defaults initExistingDockerfileDefaults) {
	if !setFlags["image"] && defaults.BaseImage != "" {
		f.image = defaults.BaseImage
	}
	if !setFlags["remote-user"] && defaults.RemoteIdentityUser != "" {
		f.remoteIdentityUser = defaults.RemoteIdentityUser
	}
	if !setFlags["remote-uid"] && defaults.RemoteIdentityUID != "" {
		f.remoteIdentityUID = defaults.RemoteIdentityUID
	}
}

// loadInitExistingDevcontainerDefaults parses .devcontainer/devcontainer.json
// when present and extracts fields relevant to decomk init prompts/defaults.
func loadInitExistingDevcontainerDefaults(repoRoot string) (initExistingDevcontainerDefaults, bool, error) {
	path := filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return initExistingDevcontainerDefaults{}, false, nil
		}
		return initExistingDevcontainerDefaults{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	stripped, err := stripJSONCLineCommentsForInit(content)
	if err != nil {
		return initExistingDevcontainerDefaults{}, false, fmt.Errorf("strip jsonc line comments from %s: %w", path, err)
	}

	var parsed struct {
		Name                string         `json:"name"`
		Image               string         `json:"image"`
		Build               map[string]any `json:"build"`
		ContainerEnv        map[string]any `json:"containerEnv"`
		RemoteUser          string         `json:"remoteUser"`
		ContainerUser       string         `json:"containerUser"`
		UpdateRemoteUserUID *bool          `json:"updateRemoteUserUID"`
	}
	if err := json.Unmarshal(stripped, &parsed); err != nil {
		return initExistingDevcontainerDefaults{}, false, fmt.Errorf("parse %s: %w", path, err)
	}

	defaults := initExistingDevcontainerDefaults{
		Name:                parsed.Name,
		Image:               parsed.Image,
		RemoteUser:          parsed.RemoteUser,
		ContainerUser:       parsed.ContainerUser,
		UpdateRemoteUserUID: parsed.UpdateRemoteUserUID,
		ConfURI:             stringValueFromAnyMap(parsed.ContainerEnv, "DECOMK_CONF_URI"),
		ToolURI:             stringValueFromAnyMap(parsed.ContainerEnv, "DECOMK_TOOL_URI"),
		Home:                stringValueFromAnyMap(parsed.ContainerEnv, "DECOMK_HOME"),
		LogDir:              stringValueFromAnyMap(parsed.ContainerEnv, "DECOMK_LOG_DIR"),
		RemoteIdentityUser:  stringValueFromAnyMap(parsed.ContainerEnv, "DECOMK_REMOTE_USER"),
		RemoteIdentityUID:   stringValueFromAnyMap(parsed.ContainerEnv, "DECOMK_REMOTE_UID"),
		FailNoBoot:          stringValueFromAnyMap(parsed.ContainerEnv, "DECOMK_FAIL_NOBOOT"),
	}
	if parsed.Build != nil {
		defaults.BuildDockerfile = stringValueFromAnyMap(parsed.Build, "dockerfile")
		defaults.BuildContext = stringValueFromAnyMap(parsed.Build, "context")
	}
	return defaults, true, nil
}

// loadInitExistingDockerfileDefaults parses .devcontainer/Dockerfile when
// present and extracts fields relevant to producer init prompts/defaults.
func loadInitExistingDockerfileDefaults(repoRoot string) (initExistingDockerfileDefaults, bool, error) {
	path := filepath.Join(repoRoot, ".devcontainer", "Dockerfile")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return initExistingDockerfileDefaults{}, false, nil
		}
		return initExistingDockerfileDefaults{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	firstFrom := ""
	remoteUser := ""
	remoteUID := ""
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		upperLine := strings.ToUpper(line)
		if strings.HasPrefix(upperLine, "FROM ") {
			if firstFrom == "" {
				baseImage, parseErr := parseDockerfileFromImage(line)
				if parseErr != nil {
					return initExistingDockerfileDefaults{}, false, fmt.Errorf("parse FROM in %s line %d: %w", path, lineNum, parseErr)
				}
				firstFrom = baseImage
			}
			continue
		}

		if strings.HasPrefix(upperLine, "ENV ") {
			assignments := parseDockerfileEnvAssignments(line)
			if value, ok := assignments["DECOMK_REMOTE_USER"]; ok && value != "" {
				remoteUser = value
			}
			if value, ok := assignments["DECOMK_REMOTE_UID"]; ok && value != "" {
				remoteUID = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return initExistingDockerfileDefaults{}, false, fmt.Errorf("scan %s: %w", path, err)
	}
	if firstFrom == "" {
		return initExistingDockerfileDefaults{}, false, fmt.Errorf("no FROM line found in %s", path)
	}

	return initExistingDockerfileDefaults{
		BaseImage:          firstFrom,
		RemoteIdentityUser: remoteUser,
		RemoteIdentityUID:  remoteUID,
	}, true, nil
}

func parseDockerfileFromImage(line string) (string, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", fmt.Errorf("missing base image")
	}

	index := 1
	for index < len(fields) {
		field := fields[index]
		if strings.HasPrefix(field, "--") {
			if strings.Contains(field, "=") {
				index++
			} else {
				index += 2
			}
			continue
		}
		return field, nil
	}

	return "", fmt.Errorf("missing base image after FROM options")
}

func parseDockerfileEnvAssignments(line string) map[string]string {
	assignments := map[string]string{}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return assignments
	}
	for _, field := range fields[1:] {
		name, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		assignments[name] = strings.Trim(value, "\"'")
	}
	return assignments
}

// stripJSONCLineCommentsForInit removes full-line // comments from devcontainer
// JSON so common JSONC files can be decoded as ordinary JSON.
func stripJSONCLineCommentsForInit(content []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func stringValueFromAnyMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
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
