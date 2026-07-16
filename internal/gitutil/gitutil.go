// Package gitutil wraps the git CLI as subprocess calls. mcsync never
// implements git logic itself; it only shells out and streams output.
package gitutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Run executes `git <args...>` in dir, streaming stdout/stderr to the
// terminal so the user sees git's own output.
func Run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Output executes `git <args...>` in dir and returns trimmed stdout.
func Output(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// IsInstalled reports whether the git executable is reachable.
func IsInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsRepo reports whether dir is inside a git working tree.
func IsRepo(dir string) bool {
	out, err := Output(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// PathExistsAtRevision reports whether path exists (as a file or
// directory) in revision's tree. Used before `git checkout <rev> -- path`,
// which errors on any pathspec absent from that revision.
func PathExistsAtRevision(dir, revision, path string) bool {
	slashPath := strings.ReplaceAll(path, "\\", "/")
	_, err := Output(dir, "cat-file", "-e", revision+":"+slashPath)
	return err == nil
}

// HasRemote reports whether the repo in dir has a remote named name configured.
func HasRemote(dir, name string) bool {
	out, err := Output(dir, "remote")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// HasChanges reports whether there are uncommitted changes (staged,
// unstaged, or untracked) for the given paths (paths may be empty to check
// the whole working tree).
func HasChanges(dir string, paths ...string) (bool, error) {
	args := append([]string{"status", "--porcelain"}, paths...)
	out, err := Output(dir, args...)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// HasStagedChanges reports whether there are staged changes ready to commit.
func HasStagedChanges(dir string) (bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return false, nil // exit 0: no staged diff
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return true, nil // exit 1: staged diff present
	}
	return false, fmt.Errorf("git diff --cached --quiet: %w", err)
}

// CurrentBranch returns the current branch name, or "" if unknown (e.g. detached HEAD).
func CurrentBranch(dir string) string {
	out, err := Output(dir, "branch", "--show-current")
	if err != nil {
		return ""
	}
	return out
}

// LFSInstalled reports whether the git-lfs extension is available. mcsync
// tracks large mod jars (data/mods) with Git LFS so a plain-git repo
// doesn't balloon every time a mod is added or updated.
func LFSInstalled() bool {
	_, err := Output("", "lfs", "version")
	return err == nil
}

// LFSInstall registers git-lfs's filters for dir's repo. Idempotent --
// safe to call on every init/setup even if already installed.
func LFSInstall(dir string) error {
	return Run(dir, "lfs", "install")
}

// LFSTrack runs `git lfs track <pattern>` in dir, creating or updating
// .gitattributes accordingly.
func LFSTrack(dir, pattern string) error {
	return Run(dir, "lfs", "track", pattern)
}

// CommitInfo returns the short hash, author date, and subject of revision
// (e.g. "HEAD", a commit hash, a tag).
func CommitInfo(dir, revision string) (hash, date, subject string, err error) {
	out, err := Output(dir, "log", "-1", "--format=%h%x1f%ai%x1f%s", revision)
	if err != nil {
		return "", "", "", err
	}
	parts := strings.SplitN(out, "\x1f", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("unexpected `git log` output for %s: %q", revision, out)
	}
	return parts[0], parts[1], parts[2], nil
}

// AheadBehind returns how many commits the current branch is ahead of and
// behind its upstream (@{u}). Errors if no upstream is configured.
func AheadBehind(dir string) (ahead, behind int, err error) {
	out, err := Output(dir, "rev-list", "--left-right", "--count", "HEAD...@{u}")
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Fields(out)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected `git rev-list` output: %q", out)
	}
	if ahead, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, err
	}
	if behind, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// LogEntry is one commit's summary, as returned by LogPaths.
type LogEntry struct {
	Hash    string
	Date    string
	Subject string
}

// LogPaths returns up to limit commits (most recent first) that touched
// any of paths.
func LogPaths(dir string, limit int, paths ...string) ([]LogEntry, error) {
	args := []string{"log", fmt.Sprintf("-%d", limit), "--format=%h%x1f%ai%x1f%s", "--"}
	args = append(args, paths...)
	out, err := Output(dir, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var entries []LogEntry
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) != 3 {
			continue
		}
		entries = append(entries, LogEntry{Hash: parts[0], Date: parts[1], Subject: parts[2]})
	}
	return entries, nil
}

// LFSPrune deletes old local Git LFS object copies from dir's
// .git/lfs/objects cache. git-lfs only prunes objects it has verified are
// already on the remote and aren't referenced by recent local history, so
// it's safe to run routinely -- nothing still-needed gets deleted. This is
// what keeps the local LFS cache from growing unbounded as mod jars are
// added/updated over time (each version briefly exists in the cache both
// before and after a push).
func LFSPrune(dir string) error {
	return Run(dir, "lfs", "prune")
}
