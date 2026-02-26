// Package makeexec runs GNU make as a subprocess.
//
// decomk intentionally execs make directly (no shell) so quoting and argument
// boundaries are deterministic and unit-testable.
package makeexec

import (
	"io"
	"os/exec"
)

// Run executes "make" in dir using the given makefile, variable tuples, and
// targets.
//
// Ordering matters:
//   - variable tuples must appear before targets on argv
//   - targets are passed exactly as provided
//
// It returns make's exit code. If make could not be started, exitCode will be 1
// and err will describe the failure.
func Run(dir, makefile string, tuples, targets []string, env []string, stdout, stderr io.Writer) (exitCode int, err error) {
	return RunWithFlags(dir, makefile, nil, tuples, targets, env, stdout, stderr)
}

// RunWithFlags executes "make" like Run, but prepends additional make flags
// (for example "-n" for a dry-run).
//
// Flags must appear before variable tuples and targets on argv so GNU make
// interprets them as options rather than goals.
func RunWithFlags(dir, makefile string, flags, tuples, targets []string, env []string, stdout, stderr io.Writer) (exitCode int, err error) {
	args := []string{}
	args = append(args, flags...)
	if makefile != "" {
		args = append(args, "-f", makefile)
	}
	args = append(args, tuples...)
	args = append(args, targets...)

	cmd := exec.Command("make", args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), err
		}
		return 1, err
	}
	return 0, nil
}
