// Package state computes decomk's on-disk layout and provides helpers for
// managing persistent state (locks, stamps, env snapshots).
//
// decomk is designed to keep its state outside the workspace repo, typically
// under a container-local directory like /var/decomk. This avoids dirtying repos
// with stamp files and makes it clearer what is "policy" (config/makefiles) vs
// "state" (stamps, logs, generated env snapshots).
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"
)

const (
	// DefaultHome is the default persistent root used inside a container when
	// DECOMK_HOME is not set.
	DefaultHome = "/var/decomk"
)

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
//   - different raw keys can sanitize to the same component and collide
//
// This function produces a *single path component* that is:
//   - ASCII-ish (letters/digits/._- with '_' substitutions)
//   - never "." or ".."
//   - collision-resistant (adds a short hash suffix of the original string)
//
// The mapping is stable but not reversible; callers should keep the original
// string separately for display/logging.
func SafeComponent(s string) string {
	if s == "" {
		s = "_"
	}

	var b strings.Builder
	b.Grow(len(s))

	lastUnderscore := false
	for _, r := range s {
		keep := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.'
		if keep {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}

	prefix := strings.Trim(b.String(), "_-")
	if prefix == "" || prefix == "." || prefix == ".." {
		prefix = "_"
	}
	const maxPrefixLen = 48
	if len(prefix) > maxPrefixLen {
		prefix = prefix[:maxPrefixLen]
	}

	sum := sha256.Sum256([]byte(s))
	suffix := hex.EncodeToString(sum[:4]) // 8 hex chars is enough to avoid accidental collisions.
	return prefix + "-" + suffix
}

// StampDir returns the stamp directory for the given workspace/context.
//
// This directory is used as make's working directory so that file targets ("stamp
// files") are created outside the workspace repo.
func StampDir(home, workspaceKey, contextKey string) string {
	return filepath.Join(home, "state", "stamps", workspaceKey, contextKey)
}

// EnvFile returns the env snapshot file path for the given workspace/context.
//
// This is a shell-friendly file containing "export NAME='value'" lines.
func EnvFile(home, workspaceKey, contextKey string) string {
	return filepath.Join(home, "state", "env", workspaceKey, contextKey+".sh")
}

// AuditDir returns the directory where logs for a single run should be written.
//
// Callers should ensure runID is unique per invocation to avoid overwriting logs.
func AuditDir(home, workspaceKey, runID string) string {
	return filepath.Join(home, "state", "audit", workspaceKey, runID)
}

// WorkspaceLockPath returns the lock file path for a workspace.
func WorkspaceLockPath(home, workspaceKey string) string {
	return filepath.Join(home, "state", "locks", workspaceKey+".lock")
}

// UpdateLockPath returns the lock file path for decomk repo/config updates.
func UpdateLockPath(home string) string {
	return filepath.Join(home, "state", "locks", "update.lock")
}

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
