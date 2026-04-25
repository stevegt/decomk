package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	checkpointSubcommandBuild = "build"
	checkpointSubcommandPush  = "push"
	checkpointSubcommandTag   = "tag"

	checkpointBuildVerboseLogLevel = "trace"
	checkpointBuildQuietLogLevel   = "info"
)

var dockerPushDigestPattern = regexp.MustCompile(`(?i)digest:\s*(sha256:[0-9a-f]{64})`)

type checkpointCommandOutput struct {
	Stdout string
	Stderr string
}

type checkpointRunner interface {
	run(name string, args ...string) (checkpointCommandOutput, error)
}

type execCheckpointRunner struct{}

func (execCheckpointRunner) run(name string, args ...string) (checkpointCommandOutput, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return checkpointCommandOutput{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, err
}

type checkpointDeps struct {
	runner checkpointRunner
	now    func() time.Time
	pid    func() int
}

func defaultCheckpointDeps() checkpointDeps {
	return checkpointDeps{
		runner: execCheckpointRunner{},
		now:    time.Now,
		pid:    os.Getpid,
	}
}

type checkpointBuildFlags struct {
	workspaceFolder string
	configPath      string
	tag             string
	keepContainer   bool
	quiet           bool
}

type checkpointPublishFlags struct {
	move bool
}

type checkpointBuildOutput struct {
	Command          string `json:"command"`
	WorkspaceFolder  string `json:"workspaceFolder"`
	DevcontainerPath string `json:"devcontainerPath"`
	ContainerID      string `json:"containerId"`
	CandidateTag     string `json:"candidateTag"`
	SourceInput      string `json:"sourceInput"`
	SourceResolved   string `json:"sourceResolved"`
	KeepContainer    bool   `json:"keepContainer"`
}

type checkpointTagResult struct {
	Destination string `json:"destination"`
	Existed     bool   `json:"existed"`
	Moved       bool   `json:"moved"`
	Status      string `json:"status"`
	Digest      string `json:"digest,omitempty"`
}

type checkpointPublishOutput struct {
	Command        string                `json:"command"`
	SourceInput    string                `json:"sourceInput"`
	SourceResolved string                `json:"sourceResolved"`
	SourcePulled   bool                  `json:"sourcePulled"`
	Move           bool                  `json:"move"`
	Tags           []checkpointTagResult `json:"tags"`
}

// cmdCheckpoint routes "decomk checkpoint" subcommands.
func cmdCheckpoint(args []string, stdout, stderr io.Writer) (int, error) {
	return cmdCheckpointWithDeps(args, stdout, stderr, defaultCheckpointDeps())
}

func cmdCheckpointWithDeps(args []string, stdout, stderr io.Writer, deps checkpointDeps) (int, error) {
	if len(args) == 0 {
		return 2, fmt.Errorf("checkpoint subcommand required\n\n%s", checkpointUsage())
	}

	switch args[0] {
	case "-h", "-help", "--help", "help":
		if err := writeLine(stdout, checkpointUsage()); err != nil {
			return 1, err
		}
		return 0, nil
	case checkpointSubcommandBuild:
		return cmdCheckpointBuild(args[1:], stdout, stderr, deps)
	case checkpointSubcommandPush:
		return cmdCheckpointPublish(args[1:], stdout, stderr, deps, checkpointSubcommandPush)
	case checkpointSubcommandTag:
		return cmdCheckpointPublish(args[1:], stdout, stderr, deps, checkpointSubcommandTag)
	default:
		return 2, fmt.Errorf("unknown checkpoint subcommand: %s\n\n%s", args[0], checkpointUsage())
	}
}

func checkpointUsage() string {
	return `decomk checkpoint - build/push/tag shared checkpoint images

Usage:
  decomk checkpoint build [flags]
  decomk checkpoint push [-m] <source> <tag...>
  decomk checkpoint tag [-m] <source> <tag...>

Subcommands:
  build
      Run devcontainer prebuild lifecycle and commit a local candidate image.
      Flags:
        -workspace-folder <path>   workspace folder for devcontainer up (default ".")
        -config <path>             devcontainer.json path relative to workspace folder unless absolute (default ".devcontainer/devcontainer.json")
        -tag <image:tag>           local candidate tag (default "decomk-checkpoint:<UTC timestamp>-<pid>")
        -keep-container            keep the prebuild container for diagnostics
        -q                         quiet mode (suppress lifecycle log output)

  push
      Publish one source image to one-or-more destination tags.

  tag
      Retag one tested source image to one-or-more destination channel tags without rebuilding.

Common push/tag flags:
  -m    allow moving destination tags that already exist`
}

func cmdCheckpointBuild(args []string, stdout, stderr io.Writer, deps checkpointDeps) (exitCode int, retErr error) {
	fs := flag.NewFlagSet("decomk checkpoint build", flag.ContinueOnError)
	fs.SetOutput(stderr)

	flags := checkpointBuildFlags{
		workspaceFolder: ".",
		configPath:      ".devcontainer/devcontainer.json",
	}
	fs.StringVar(&flags.workspaceFolder, "workspace-folder", flags.workspaceFolder, "workspace folder for devcontainer up")
	fs.StringVar(&flags.configPath, "config", flags.configPath, "devcontainer.json path relative to workspace folder unless absolute")
	fs.StringVar(&flags.tag, "tag", "", "local candidate image tag (default decomk-checkpoint:<UTC timestamp>-<pid>)")
	fs.BoolVar(&flags.keepContainer, "keep-container", false, "keep checkpoint container instead of removing it after commit")
	fs.BoolVar(&flags.quiet, "q", false, "quiet mode (suppress lifecycle log output)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}
	if len(fs.Args()) != 0 {
		return 2, fmt.Errorf("build does not accept positional args: %q", strings.Join(fs.Args(), " "))
	}

	workspaceFolder, err := filepath.Abs(flags.workspaceFolder)
	if err != nil {
		return 1, fmt.Errorf("abs workspace folder %q: %w", flags.workspaceFolder, err)
	}
	devcontainerPath := flags.configPath
	if !filepath.IsAbs(devcontainerPath) {
		devcontainerPath = filepath.Join(workspaceFolder, devcontainerPath)
	}
	if _, err := os.Stat(devcontainerPath); err != nil {
		return 1, fmt.Errorf("stat devcontainer config %q: %w", devcontainerPath, err)
	}

	if strings.TrimSpace(flags.tag) == "" {
		flags.tag = defaultCheckpointTag(deps.now(), deps.pid())
	}
	if strings.TrimSpace(flags.tag) == "" {
		return 1, fmt.Errorf("checkpoint candidate tag cannot be empty")
	}

	var containerID string
	defer func() {
		if containerID == "" {
			return
		}
		if flags.keepContainer {
			return
		}
		// Intent: Make checkpoint build cleanup explicit and fail-fast so a "success"
		// result never hides leaked prebuild containers unless the operator asked to
		// retain them for diagnostics.
		// Source: DI-011-20260420-162554 (TODO/011)
		if _, err := runCheckpointCommand(deps.runner, "docker", "rm", "-f", containerID); err != nil {
			wrapped := fmt.Errorf("remove checkpoint container %s: %w", containerID, err)
			if retErr == nil {
				retErr = wrapped
				if exitCode == 0 {
					exitCode = 1
				}
				return
			}
			retErr = errors.Join(retErr, wrapped)
			return
		}
	}()

	devcontainerOut, err := runCheckpointCommand(
		deps.runner,
		"devcontainer",
		"up",
		"--workspace-folder", workspaceFolder,
		"--config", devcontainerPath,
		"--prebuild",
		"--log-level", checkpointBuildLogLevel(flags.quiet),
		"--log-format", "json",
	)
	if err != nil {
		return 1, err
	}
	if !flags.quiet {
		// Intent: Make checkpoint-build lifecycle troubleshooting visible by
		// default while keeping machine-readable checkpoint JSON on stdout.
		// Source: DI-011-20260424-160516 (TODO/011)
		if err := writeCheckpointBuildLogs(stderr, devcontainerOut); err != nil {
			return 1, err
		}
	}
	containerID, err = parseContainerIDFromDevcontainerOutput(devcontainerOut.Stdout)
	if err != nil {
		return 1, err
	}
	if strings.TrimSpace(containerID) == "" {
		return 1, fmt.Errorf("devcontainer up did not report containerId")
	}

	if _, err := runCheckpointCommand(deps.runner, "docker", "commit", containerID, flags.tag); err != nil {
		return 1, fmt.Errorf("commit checkpoint image %q from container %q: %w", flags.tag, containerID, err)
	}

	sourceResolved, err := inspectImageID(deps.runner, flags.tag)
	if err != nil {
		return 1, fmt.Errorf("inspect checkpoint source %q: %w", flags.tag, err)
	}

	reportedContainerID := containerID
	if !flags.keepContainer {
		if _, err := runCheckpointCommand(deps.runner, "docker", "rm", "-f", containerID); err != nil {
			return 1, fmt.Errorf("remove checkpoint container %s: %w", containerID, err)
		}
		containerID = ""
	}

	out := checkpointBuildOutput{
		Command:          checkpointSubcommandBuild,
		WorkspaceFolder:  workspaceFolder,
		DevcontainerPath: devcontainerPath,
		ContainerID:      reportedContainerID,
		CandidateTag:     flags.tag,
		SourceInput:      flags.tag,
		SourceResolved:   sourceResolved,
		KeepContainer:    flags.keepContainer,
	}
	if err := writeCheckpointJSON(stdout, out); err != nil {
		return 1, err
	}
	return 0, nil
}

func checkpointBuildLogLevel(quiet bool) string {
	if quiet {
		return checkpointBuildQuietLogLevel
	}
	return checkpointBuildVerboseLogLevel
}

// writeCheckpointBuildLogs writes captured devcontainer lifecycle logs to
// stderr in checkpoint-build verbose mode.
//
// Intent: Keep checkpoint artifacts deterministic on stdout while making verbose
// lifecycle logs directly visible to operators on stderr.
// Source: DI-011-20260424-160516 (TODO/011)
func writeCheckpointBuildLogs(stderr io.Writer, out checkpointCommandOutput) error {
	if err := writeCheckpointLogStream(stderr, out.Stdout); err != nil {
		return err
	}
	if err := writeCheckpointLogStream(stderr, out.Stderr); err != nil {
		return err
	}
	return nil
}

func writeCheckpointLogStream(w io.Writer, stream string) error {
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return nil
	}
	if err := writeLine(w, stream); err != nil {
		return err
	}
	return nil
}

func cmdCheckpointPublish(args []string, stdout, stderr io.Writer, deps checkpointDeps, mode string) (int, error) {
	fs := flag.NewFlagSet("decomk checkpoint "+mode, flag.ContinueOnError)
	fs.SetOutput(stderr)

	flags := checkpointPublishFlags{}
	fs.BoolVar(&flags.move, "m", false, "allow moving existing destination tags")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}

	if len(fs.Args()) < 2 {
		return 2, fmt.Errorf("%s requires <source> <tag...>", mode)
	}

	sourceInput := strings.TrimSpace(fs.Arg(0))
	if sourceInput == "" {
		return 2, fmt.Errorf("%s source cannot be empty", mode)
	}
	destinations := append([]string(nil), fs.Args()[1:]...)
	if err := validateDestinationTags(destinations); err != nil {
		return 2, err
	}

	sourceResolved, sourcePulled, err := resolveSourceReference(deps.runner, sourceInput)
	if err != nil {
		return 1, err
	}

	tagResults, err := publishSourceToTags(deps.runner, sourceInput, destinations, flags.move, mode)
	if err != nil {
		return 1, err
	}

	out := checkpointPublishOutput{
		Command:        mode,
		SourceInput:    sourceInput,
		SourceResolved: sourceResolved,
		SourcePulled:   sourcePulled,
		Move:           flags.move,
		Tags:           tagResults,
	}
	if err := writeCheckpointJSON(stdout, out); err != nil {
		return 1, err
	}
	return 0, nil
}

func validateDestinationTags(destinations []string) error {
	seen := map[string]struct{}{}
	for _, tag := range destinations {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			return fmt.Errorf("destination tags cannot include empty values")
		}
		if _, ok := seen[trimmed]; ok {
			return fmt.Errorf("destination tags must be unique; duplicate %q", trimmed)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}

func defaultCheckpointTag(now time.Time, pid int) string {
	return fmt.Sprintf("decomk-checkpoint:%s-%d", now.UTC().Format("20060102t150405z"), pid)
}

func parseContainerIDFromDevcontainerOutput(stdout string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	containerID := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		raw, ok := obj["containerId"]
		if !ok {
			continue
		}
		id, ok := raw.(string)
		if !ok || strings.TrimSpace(id) == "" {
			continue
		}
		containerID = id
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan devcontainer output: %w", err)
	}
	if containerID == "" {
		return "", fmt.Errorf("devcontainer output did not contain any JSON line with containerId")
	}
	return containerID, nil
}

func writeCheckpointJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func runCheckpointCommand(r checkpointRunner, name string, args ...string) (checkpointCommandOutput, error) {
	out, err := r.run(name, args...)
	if err != nil {
		msg := formatCommand(name, args)
		compact := compactCommandOutput(out)
		if compact == "" {
			return out, fmt.Errorf("%s: %w", msg, err)
		}
		return out, fmt.Errorf("%s: %w: %s", msg, err, compact)
	}
	return out, nil
}

func formatCommand(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return fmt.Sprintf("%s %s", name, strings.Join(args, " "))
}

func compactCommandOutput(out checkpointCommandOutput) string {
	parts := []string{}
	if s := strings.TrimSpace(out.Stdout); s != "" {
		parts = append(parts, "stdout="+truncateForError(s))
	}
	if s := strings.TrimSpace(out.Stderr); s != "" {
		parts = append(parts, "stderr="+truncateForError(s))
	}
	return strings.Join(parts, " ")
}

func truncateForError(raw string) string {
	const maxLen = 240
	if len(raw) <= maxLen {
		return raw
	}
	return raw[:maxLen] + "...(truncated)"
}

func inspectImageID(r checkpointRunner, source string) (string, error) {
	out, err := runCheckpointCommand(r, "docker", "image", "inspect", "--format", "{{.Id}}", source)
	if err != nil {
		return "", err
	}
	imageID := strings.TrimSpace(out.Stdout)
	if imageID == "" {
		return "", fmt.Errorf("docker image inspect returned empty image id for source %q", source)
	}
	return imageID, nil
}

func resolveSourceReference(r checkpointRunner, source string) (resolved string, pulled bool, err error) {
	resolved, err = inspectImageID(r, source)
	if err == nil {
		return resolved, false, nil
	}

	// Intent: Accept digest/ref/image-id source inputs uniformly by trying local
	// inspection first, then a pull fallback, so checkpoint push/tag can consume
	// either local candidates or remote-tested references.
	// Source: DI-011-20260420-162554 (TODO/011)
	if _, pullErr := runCheckpointCommand(r, "docker", "pull", source); pullErr != nil {
		return "", false, fmt.Errorf("resolve source %q: inspect failed (%v), pull failed: %w", source, err, pullErr)
	}

	resolved, err = inspectImageID(r, source)
	if err != nil {
		return "", true, fmt.Errorf("resolve source %q after pull: %w", source, err)
	}
	return resolved, true, nil
}

func publishSourceToTags(r checkpointRunner, source string, destinations []string, move bool, mode string) ([]checkpointTagResult, error) {
	results := make([]checkpointTagResult, 0, len(destinations))
	for _, destination := range destinations {
		exists, err := destinationTagExists(r, destination)
		if err != nil {
			return nil, fmt.Errorf("check destination tag %q: %w", destination, err)
		}

		// Intent: Require explicit `-m` before moving existing channel tags so
		// checkpoint rollout stays deliberate and does not overwrite tags silently.
		// Source: DI-011-20260420-162554 (TODO/011)
		if exists && !move {
			return nil, fmt.Errorf("destination tag %q already exists; rerun with -m to move it", destination)
		}

		if _, err := runCheckpointCommand(r, "docker", "tag", source, destination); err != nil {
			return nil, fmt.Errorf("tag source %q as %q: %w", source, destination, err)
		}

		pushOut, err := runCheckpointCommand(r, "docker", "push", destination)
		if err != nil {
			return nil, fmt.Errorf("push destination tag %q: %w", destination, err)
		}

		status := "pushed"
		if mode == checkpointSubcommandTag {
			status = "retagged"
		}
		results = append(results, checkpointTagResult{
			Destination: destination,
			Existed:     exists,
			Moved:       exists && move,
			Status:      status,
			Digest:      parsePushDigest(pushOut),
		})
	}
	return results, nil
}

func destinationTagExists(r checkpointRunner, destination string) (bool, error) {
	out, err := r.run("docker", "manifest", "inspect", destination)
	if err == nil {
		return true, nil
	}

	combined := strings.ToLower(strings.TrimSpace(out.Stdout + "\n" + out.Stderr))
	if strings.Contains(combined, "'manifest' is not a docker command") || strings.Contains(combined, "unknown command \"manifest\"") {
		return destinationTagExistsViaPull(r, destination)
	}
	if isRegistryNotFoundOutput(combined) {
		return false, nil
	}

	compact := compactCommandOutput(out)
	if compact == "" {
		return false, fmt.Errorf("docker manifest inspect %q: %w", destination, err)
	}
	return false, fmt.Errorf("docker manifest inspect %q: %w: %s", destination, err, compact)
}

func destinationTagExistsViaPull(r checkpointRunner, destination string) (bool, error) {
	out, err := r.run("docker", "pull", destination)
	if err == nil {
		return true, nil
	}
	combined := strings.ToLower(strings.TrimSpace(out.Stdout + "\n" + out.Stderr))
	if isRegistryNotFoundOutput(combined) {
		return false, nil
	}
	compact := compactCommandOutput(out)
	if compact == "" {
		return false, fmt.Errorf("docker pull %q while checking destination tag existence: %w", destination, err)
	}
	return false, fmt.Errorf("docker pull %q while checking destination tag existence: %w: %s", destination, err, compact)
}

func isRegistryNotFoundOutput(text string) bool {
	phrases := []string{
		"manifest unknown",
		"no such manifest",
		"name unknown",
		"not found",
	}
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func parsePushDigest(out checkpointCommandOutput) string {
	combined := out.Stdout + "\n" + out.Stderr
	match := dockerPushDigestPattern.FindStringSubmatch(combined)
	if len(match) < 2 {
		return ""
	}
	return strings.ToLower(match[1])
}
