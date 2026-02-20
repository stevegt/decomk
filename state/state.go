// Package state computes decomk's on-disk layout and provides small helpers for
// managing persistent state (locks, stamps, env snapshots).
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

func validateAbs(path, label string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path (got %q)", label, path)
	}
	return path, nil
}

// WorkspaceRoot returns the git repo toplevel directory if available; otherwise
// it returns an absolute version of startDir.
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
// The intent is "stable enough" within a container while avoiding collisions.
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
// It is intended for keys that may contain '/' (e.g., "owner/repo").
// The mapping is stable but not reversible; if this becomes ambiguous, we can
// switch to a hash-based encoding.
func SafeComponent(s string) string {
	if s == "" {
		return "_"
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
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "_"
	}
	return out
}

// StampDir returns the stamp directory for the given workspace/context.
func StampDir(home, workspaceKey, contextKey string) string {
	return filepath.Join(home, "state", "stamps", workspaceKey, contextKey)
}

// EnvFile returns the env snapshot file path for the given workspace/context.
func EnvFile(home, workspaceKey, contextKey string) string {
	return filepath.Join(home, "state", "env", workspaceKey, contextKey+".sh")
}

// AuditDir returns the directory where logs for a single run should be written.
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
type Lock struct {
	f *os.File
}

// LockFile opens and locks lockPath, creating it if needed.
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
