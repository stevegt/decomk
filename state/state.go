// Package state computes decomk's on-disk layout and provides helpers for
// managing persistent state (locks, stamps, env exports).
//
// decomk is designed to keep its state outside any workspace repo. Most state
// lives under a container-local directory like /var/decomk, while per-run logs
// are written under /var/log/decomk by default (see DefaultLogDir).
//
// Keeping state/logs out of workspaces avoids dirtying repos with stamp files and
// makes it clearer what is "policy" vs "state":
//   - /var/decomk/conf    : local clone of the shared config repo (decomk.conf + Makefile)
//   - /var/decomk/stamps  : global stamp directory used as make's working directory
//   - /var/decomk/env.sh  : shell-friendly resolved tuple exports for other processes to source
//   - /var/log/decomk     : per-run logs (make output)
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultHome is the default persistent root used inside a container when
	// DECOMK_HOME is not set.
	DefaultHome = "/var/decomk"

	// DefaultLogDir is the preferred default directory for decomk's per-run logs.
	//
	// Per-run logs intentionally live under /var/log so they can be managed
	// separately from decomk's mutable state under DefaultHome (or DECOMK_HOME).
	DefaultLogDir = "/var/log/decomk"
)

// ToolDir returns the directory where decomk keeps a clone of its own tool repo.
//
// This supports an isconf-like "self-update" model: decomk can `git pull` its
// own repo and rebuild itself before doing any other work.
func ToolDir(home string) string { return filepath.Join(home, "decomk") }

// ToolBinPath returns the path where decomk expects to build the tool binary
// from ToolDir.
//
// Keeping the binary inside ToolDir keeps the installation self-contained, but
// still outside any WIP workspace repo.
func ToolBinPath(home string) string { return filepath.Join(ToolDir(home), "bin", "decomk") }

// ToolLockPath returns the lock file used to serialize tool repo updates.
//
// The lock path is outside ToolDir so we don't create update artifacts inside a
// git working tree.
func ToolLockPath(home string) string { return filepath.Join(home, "decomk.lock") }

// ConfDir returns the directory where decomk expects the config repo clone.
//
// This is a git working tree that typically contains:
//   - decomk.conf
//   - optional decomk.d/*.conf overlays
//   - Makefile
func ConfDir(home string) string { return filepath.Join(home, "conf") }

// ConfLockPath returns the lock file used to prevent concurrent updates to the
// config repo clone.
//
// Important: this lock file must *not* live inside ConfDir, because ConfDir is
// a git working tree. Creating lock artifacts inside it would:
//   - interfere with "git clone <url> <ConfDir>" when ConfDir does not yet exist,
//   - and/or dirty the working tree.
//
// Instead we keep the lock as a sibling of ConfDir under the decomk home root.
func ConfLockPath(home string) string { return filepath.Join(home, "conf.lock") }

// StampsDir returns the global stamp directory where decomk runs make.
func StampsDir(home string) string { return filepath.Join(home, "stamps") }

// StampsLockPath returns the lock file used to prevent concurrent stamp
// mutation.
//
// Stamps are global (container-wide), so the lock is also global.
func StampsLockPath(home string) string { return filepath.Join(StampsDir(home), ".lock") }

// LogDir returns the per-run log directory under decomk's state root.
//
// This is the legacy/home-rooted location (<DECOMK_HOME>/log). Callers that want
// system-style log placement should prefer DefaultLogDir, and may fall back to
// LogDir(home) when DefaultLogDir is not writable.
func LogDir(home string) string { return filepath.Join(home, "log") }

// Home resolves the decomk home directory.
//
// Precedence:
//   - flagOverride (if non-empty)
//   - DECOMK_HOME
//   - /var/decomk
func Home(flagOverride string) (string, error) {
	if flagOverride != "" {
		return validateAbs(flagOverride, "flag -home")
	}
	if env := os.Getenv("DECOMK_HOME"); env != "" {
		return validateAbs(env, "DECOMK_HOME")
	}
	return DefaultHome, nil
}

// validateAbs ensures a path is absolute so callers never accidentally create
// state relative to the current working directory (which could be inside a repo).
func validateAbs(path, label string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path (got %q)", label, path)
	}
	return path, nil
}

// WorkspaceRoot returns the workspace root directory.
//
// If startDir is inside a git repo, this returns "git rev-parse --show-toplevel".
// Otherwise it returns an absolute version of startDir.
func WorkspaceRoot(startDir string) (string, error) {
	if startDir == "" {
		startDir = "."
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = startDir
	out, err := cmd.Output()
	if err == nil {
		root := strings.TrimSpace(string(out))
		if root != "" {
			return root, nil
		}
	}
	return filepath.Abs(startDir)
}

// WorkspaceKey returns a filesystem-safe identifier for the workspace.
//
// We intentionally avoid using the raw path as a directory component:
//   - it may contain '/' and other characters
//   - it may leak host filesystem structure in logs/state
//
// Instead we hash the workspace root (and optionally the GitHub repo identifier)
// into a stable per-workspace key.
func WorkspaceKey(workspaceRoot, githubRepo string) (string, error) {
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(githubRepo + "\n" + abs))
	return hex.EncodeToString(h[:]), nil
}

// SafeComponent returns a filesystem-safe path component derived from s.
//
// decomk uses user-controlled strings (context keys like "owner/repo") as part
// of on-disk state paths. Using them directly would be a correctness and
// security hazard:
//   - path separators could create nested paths
//   - "." / ".." could escape the intended directory
//
// This function produces a *single path component* by percent-encoding the
// input (URL path escaping). This keeps the mapping stable and unambiguous, and
// avoids collisions that can occur with simple "replace non-alnum with '_'"
// sanitizers.
func SafeComponent(s string) string {
	if s == "" {
		s = "_"
	}

	escaped := url.PathEscape(s)
	if escaped == "" || escaped == "." || escaped == ".." {
		escaped = "_" + escaped
	}
	return escaped
}

// StampDir returns decomk's stamp directory.
//
// Stamps are global (container-scoped) because decomk is intended to configure
// the container based on what's in /workspaces, not to configure the repos
// themselves.
func StampDir(home string) string { return StampsDir(home) }

// EnvFile returns the env export file path.
//
// This file is intentionally stable so other processes can source it after
// running decomk. It is overwritten on each invocation.
func EnvFile(home string) string { return filepath.Join(home, "env.sh") }

// EnsureDir ensures a directory exists with safe permissions.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// EnsureParentDir ensures path's parent directory exists.
func EnsureParentDir(path string) error {
	return EnsureDir(filepath.Dir(path))
}

// Lock is an advisory file lock held via flock(2).
//
// This is intended to prevent concurrent decomk invocations from mutating the
// same state directories at the same time.
type Lock struct {
	f *os.File
}

// LockFile opens and exclusively locks lockPath, creating it if needed.
//
// The lock is blocking: callers will wait until the lock becomes available.
func LockFile(lockPath string) (*Lock, error) {
	if err := EnsureParentDir(lockPath); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Close unlocks and closes the lock file.
func (l *Lock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}

// TouchExistingStamps updates the mtime of existing, non-hidden files in stampDir.
//
// This mirrors the isconf/lunamake "touch *" behavior: stamps are treated as
// explicit invalidation artifacts (delete to rerun).
//
// Only regular files in stampDir are touched:
//   - no recursion
//   - hidden files (".*") are ignored
//   - non-regular files are ignored
func TouchExistingStamps(stampDir string, now time.Time) error {
	entries, err := os.ReadDir(stampDir)
	if err != nil {
		// If the directory doesn't exist yet, there's nothing to touch.
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		p := filepath.Join(stampDir, name)
		if err := os.Chtimes(p, now, now); err != nil {
			return err
		}
	}
	return nil
}
