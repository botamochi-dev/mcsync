package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mcsync/internal/gitutil"
)

// lockFileName is a small tracked file recording which PC currently has
// the server running, so `start` can warn instead of silently racing
// another PC's in-progress session. It's committed/pushed immediately by
// start and removed by stop, independent of the world/config/mods save
// cycle (see saveTrackedState).
const lockFileName = ".mcsync-lock"

type lockInfo struct {
	Host      string    `json:"host"`
	User      string    `json:"user"`
	StartedAt time.Time `json:"startedAt"`
}

func lockPath(dir string) string {
	return filepath.Join(dir, lockFileName)
}

func readLock(dir string) (*lockInfo, error) {
	data, err := os.ReadFile(lockPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var info lockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", lockFileName, err)
	}
	return &info, nil
}

// checkLock errors out if another PC's lock is currently active, unless
// force is set. A lock left over from this same PC (e.g. after a crash or
// an unclean shutdown) is treated as stale and doesn't block startup.
func checkLock(dir string, force bool) error {
	info, err := readLock(dir)
	if err != nil || info == nil {
		return nil
	}
	host, _ := os.Hostname()
	if info.Host == host {
		fmt.Printf("Note: found a leftover lock from this PC (started %s) -- probably an unclean shutdown last time. Continuing.\n",
			info.StartedAt.Format("2006-01-02 15:04"))
		return nil
	}
	if !force {
		return fmt.Errorf("another PC (%s%s) may still be running this server, started %s.\n"+
			"Starting anyway risks overwriting their progress when they save.\n"+
			"Make sure it's really stopped there, then retry with --force to override",
			info.Host, userSuffix(info.User), info.StartedAt.Format("2006-01-02 15:04"))
	}
	fmt.Printf("Warning: overriding lock held by %s (--force)\n", info.Host)
	return nil
}

func userSuffix(user string) string {
	if user == "" {
		return ""
	}
	return fmt.Sprintf(" / %s", user)
}

// writeLockAndCommit records that this PC is now running the server, and
// commits+pushes it immediately (separate from the world/config/mods save
// cycle) so other PCs see it as soon as they next pull.
func writeLockAndCommit(dir string) error {
	host, _ := os.Hostname()
	user, _ := gitutil.Output(dir, "config", "user.name")
	info := lockInfo{Host: host, User: user, StartedAt: time.Now()}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(lockPath(dir), data, 0o644); err != nil {
		return err
	}
	if err := gitutil.Run(dir, "add", lockFileName); err != nil {
		return err
	}
	changed, err := gitutil.HasStagedChanges(dir)
	if err != nil || !changed {
		return err
	}
	if err := gitutil.Run(dir, "commit", "-m", fmt.Sprintf("chore: lock server on %s", host)); err != nil {
		return err
	}
	return gitutil.Run(dir, "push")
}

// removeLockAndStage deletes the lock file (if it belongs to this PC) and
// returns its path for inclusion in the caller's next `git add`. A lock
// held by a different PC is left untouched.
func removeLockAndStage(dir string) ([]string, error) {
	info, err := readLock(dir)
	if err != nil || info == nil {
		return nil, nil
	}
	host, _ := os.Hostname()
	if info.Host != host {
		fmt.Printf("Note: lock is held by a different PC (%s); leaving it in place.\n", info.Host)
		return nil, nil
	}
	if err := os.Remove(lockPath(dir)); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return []string{lockFileName}, nil
}
