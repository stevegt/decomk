// Package makeexec runs GNU make as a subprocess.
package makeexec

import (
	"io"
	"os/exec"
)

// Run executes "make" in dir using the given makefile, variable tuples, and targets.
//
// It returns make's exit code. If make could not be started, exitCode will be 1
// and err will describe the failure.
func Run(dir, makefile string, tuples, targets []string, env []string, stdout, stderr io.Writer) (exitCode int, err error) {
	args := []string{}
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
