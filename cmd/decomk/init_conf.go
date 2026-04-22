package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/stevegt/decomk/confrepo"
	"github.com/stevegt/decomk/stage0"
)

// initConfFlags are user-facing options for "decomk init-conf".
type initConfFlags struct {
	repoRoot string
	name     string
	confURI  string
	toolURI  string
	home     string
	logDir   string
	runArgs  string
	force    bool
}

// cmdInitConf writes shared config-repo starter files in the target repo root.
//
// Intent: Provide first-class conf repo bootstrap so teams can create a shared
// decomk policy repo (decomk.conf + Makefile + producer devcontainer) without
// manual copy/paste.
// Source: DI-013-20260422-110500 (TODO/013)
func cmdInitConf(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("decomk init-conf", flag.ContinueOnError)
	fs.SetOutput(stderr)

	flags := initConfFlags{
		repoRoot: "",
		name:     confrepo.DefaultName,
		confURI:  confrepo.DefaultConfURI,
		toolURI:  stage0.DefaultToolURI,
		home:     confrepo.DefaultHome,
		logDir:   confrepo.DefaultLogDir,
		runArgs:  confrepo.DefaultRunArgs,
	}
	addInitConfFlags(fs, &flags)

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

	repoRootInput := flags.repoRoot
	var err error
	if !setFlags["repo-root"] {
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

	if strings.TrimSpace(flags.name) == "" {
		flags.name = confrepo.DefaultName
	}
	if strings.TrimSpace(flags.confURI) == "" {
		return 1, fmt.Errorf("DECOMK_CONF_URI template value cannot be empty")
	}
	if !strings.HasPrefix(flags.confURI, "git:") {
		return 1, fmt.Errorf("DECOMK_CONF_URI template value must start with git: (got %q)", flags.confURI)
	}
	if strings.TrimSpace(flags.toolURI) == "" {
		return 1, fmt.Errorf("DECOMK_TOOL_URI template value cannot be empty")
	}
	if !strings.HasPrefix(flags.toolURI, "go:") && !strings.HasPrefix(flags.toolURI, "git:") {
		return 1, fmt.Errorf("DECOMK_TOOL_URI template value must start with go: or git: (got %q)", flags.toolURI)
	}
	if flags.home == "" || !filepath.IsAbs(flags.home) {
		return 1, fmt.Errorf("DECOMK_HOME template value must be an absolute path (got %q)", flags.home)
	}
	if flags.logDir == "" || !filepath.IsAbs(flags.logDir) {
		return 1, fmt.Errorf("DECOMK_LOG_DIR template value must be an absolute path (got %q)", flags.logDir)
	}
	if strings.TrimSpace(flags.runArgs) == "" {
		return 1, fmt.Errorf("DECOMK_RUN_ARGS template value cannot be empty")
	}

	data := confrepo.ProducerDevcontainerData(flags.name)
	data.ConfURI = flags.confURI
	data.ToolURI = flags.toolURI
	data.Home = flags.home
	data.LogDir = flags.logDir
	data.DecomkRunArgs = flags.runArgs

	results, err := writeInitConfScaffold(repoRoot, data, flags.force)
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

// addInitConfFlags defines flags for "decomk init-conf".
func addInitConfFlags(fs *flag.FlagSet, flags *initConfFlags) {
	fs.StringVar(&flags.repoRoot, "repo-root", flags.repoRoot, "target conf repo root (default: current git repo root)")
	fs.StringVar(&flags.name, "name", flags.name, "devcontainer name value")
	fs.StringVar(&flags.confURI, "conf-uri", flags.confURI, "DECOMK_CONF_URI value for generated producer devcontainer")
	fs.StringVar(&flags.toolURI, "tool-uri", flags.toolURI, "DECOMK_TOOL_URI value for generated producer devcontainer")
	fs.StringVar(&flags.home, "home", flags.home, "DECOMK_HOME value for generated producer devcontainer")
	fs.StringVar(&flags.logDir, "log-dir", flags.logDir, "DECOMK_LOG_DIR value for generated producer devcontainer")
	fs.StringVar(&flags.runArgs, "run-args", flags.runArgs, "DECOMK_RUN_ARGS value for generated producer devcontainer")
	fs.BoolVar(&flags.force, "force", false, "overwrite existing conf-repo scaffold files")
	fs.BoolVar(&flags.force, "f", false, "alias for -force")
}

// writeInitConfScaffold renders and writes all managed conf-repo scaffold
// files.
func writeInitConfScaffold(repoRoot string, data stage0.DevcontainerTemplateData, force bool) ([]initWriteResult, error) {
	devcontainerData := data.EnsureDefaults()

	decomkConf, err := stage0.RenderTemplate("confrepo.decomk.conf", initConfRepoDecomkConfTemplate, struct{}{})
	if err != nil {
		return nil, err
	}
	makefile, err := stage0.RenderTemplate("confrepo.Makefile", initConfRepoMakefileTemplate, struct{}{})
	if err != nil {
		return nil, err
	}
	readme, err := stage0.RenderTemplate("confrepo.README.md", initConfRepoREADMETemplate, struct{}{})
	if err != nil {
		return nil, err
	}
	helloScript, err := stage0.RenderTemplate("confrepo.hello-world.sh", initConfRepoHelloWorldTemplate, struct{}{})
	if err != nil {
		return nil, err
	}
	devcontainerJSON, err := stage0.RenderTemplate("confrepo.devcontainer.json", initConfRepoDevcontainerJSONTemplate, devcontainerData)
	if err != nil {
		return nil, err
	}
	stage0Script, err := stage0.RenderStage0Script(initStage0ScriptTemplate)
	if err != nil {
		return nil, err
	}
	dockerfile, err := stage0.RenderTemplate("confrepo.Dockerfile", initConfRepoDockerfileTemplate, struct{}{})
	if err != nil {
		return nil, err
	}

	type outputFile struct {
		relPath string
		content []byte
		mode    os.FileMode
	}
	files := []outputFile{
		{relPath: "decomk.conf", content: decomkConf, mode: 0o644},
		{relPath: "Makefile", content: makefile, mode: 0o644},
		{relPath: "README.md", content: readme, mode: 0o644},
		{relPath: filepath.Join("bin", "hello-world.sh"), content: helloScript, mode: 0o755},
		{relPath: filepath.Join(".devcontainer", "devcontainer.json"), content: devcontainerJSON, mode: 0o644},
		{relPath: filepath.Join(".devcontainer", "decomk-stage0.sh"), content: stage0Script, mode: 0o755},
		{relPath: filepath.Join(".devcontainer", "Dockerfile"), content: dockerfile, mode: 0o644},
	}

	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, filepath.Join(repoRoot, file.relPath))
	}
	if !force {
		// Intent: Keep conf-repo initialization conservative by default and force
		// explicit operator acknowledgement before overwriting managed starter files.
		// Source: DI-013-20260422-110500 (TODO/013)
		existing, err := existingInitTargets(paths...)
		if err != nil {
			return nil, err
		}
		if len(existing) > 0 {
			return nil, initConfExistingTargetsError(existing)
		}
	}

	results := make([]initWriteResult, 0, len(files))
	for _, file := range files {
		absPath := filepath.Join(repoRoot, file.relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, err
		}
		status, err := writeInitFile(absPath, file.content, file.mode, force)
		if err != nil {
			return nil, err
		}
		results = append(results, initWriteResult{Path: absPath, Status: status})
	}

	return results, nil
}

// initConfExistingTargetsError formats strict non-force guidance for existing
// conf-repo scaffold files.
func initConfExistingTargetsError(existing []string) error {
	var builder strings.Builder
	builder.WriteString("refusing to overwrite existing conf-repo scaffold file(s) without -f/-force:\n")
	for _, path := range existing {
		builder.WriteString("  - ")
		builder.WriteString(path)
		builder.WriteString("\n")
	}
	builder.WriteString("recommended workflow:\n")
	builder.WriteString("  1) git commit the current files\n")
	builder.WriteString("  2) run decomk init-conf -f ...\n")
	builder.WriteString("  3) review and merge with git difftool\n")
	return errors.New(builder.String())
}

