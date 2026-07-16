package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"mcsync/internal/dockerutil"
	"mcsync/internal/gitutil"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show server state, save history, and repo health at a glance",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(c *cobra.Command, args []string) error {
	dir := "."

	fmt.Println("Server:")
	printServerStatus(dir)

	fmt.Println("\nGit:")
	if !gitutil.IsInstalled() || !gitutil.IsRepo(dir) {
		fmt.Println("  not a git repo (run `mcsync init` first)")
	} else {
		printGitStatus(dir)
		printLockStatus(dir)
	}

	fmt.Println("\nDisk usage:")
	printSize("  data/world", dirSize(filepath.Join(dir, "data", "world")))
	gitSize := dirSize(filepath.Join(dir, ".git"))
	lfsSize := dirSize(filepath.Join(dir, ".git", "lfs", "objects"))
	printSize("  .git", gitSize)
	if lfsSize > 0 {
		printSize("    of which LFS cache", lfsSize)
	}

	return nil
}

func printServerStatus(dir string) {
	if !dockerutil.DaemonRunning() {
		fmt.Println("  Docker daemon isn't running")
		return
	}
	state, health, err := dockerutil.ServiceStatus(dir, "mc")
	if err != nil {
		fmt.Println("  not running")
		return
	}
	if health != "" {
		fmt.Printf("  %s (%s)\n", state, health)
	} else {
		fmt.Printf("  %s\n", state)
	}
}

func printGitStatus(dir string) {
	hash, date, subject, err := gitutil.CommitInfo(dir, "HEAD")
	if err == nil {
		fmt.Printf("  Last save: %s  %s  (%s)\n", date, subject, hash)
	}

	// Scoped to the paths mcsync actually manages, so a stray unrelated
	// file sitting in the project folder doesn't get reported as an
	// "uncommitted change" to the server's saved state.
	relevantPaths := append([]string{"docker-compose.yml", ".gitignore", ".gitattributes"}, trackedStatePaths...)
	dirty, err := gitutil.HasChanges(dir, relevantPaths...)
	if err == nil && dirty {
		fmt.Println("  Uncommitted changes present (run `mcsync stop` or `mcsync mods add` to save them)")
	}

	if !gitutil.HasRemote(dir, "origin") {
		fmt.Println("  no remote configured")
		return
	}
	ahead, behind, err := gitutil.AheadBehind(dir)
	if err != nil {
		fmt.Println("  remote configured, but couldn't compare against it (no upstream tracking branch?)")
		return
	}
	switch {
	case ahead > 0 && behind > 0:
		fmt.Printf("  %d commit(s) ahead, %d behind origin -- push and pull needed\n", ahead, behind)
	case ahead > 0:
		fmt.Printf("  %d commit(s) not yet pushed\n", ahead)
	case behind > 0:
		fmt.Printf("  %d commit(s) behind origin -- run `mcsync start` to pull\n", behind)
	default:
		fmt.Println("  up to date with origin")
	}
}

func printLockStatus(dir string) {
	info, err := readLock(dir)
	if err != nil || info == nil {
		fmt.Println("  Lock: none (no PC currently marked as running this server)")
		return
	}
	host, _ := os.Hostname()
	who := info.Host
	if info.Host == host {
		who += " (this PC)"
	}
	fmt.Printf("  Lock: held by %s%s since %s\n", who, userSuffix(info.User), info.StartedAt.Format("2006-01-02 15:04"))
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func printSize(label string, bytes int64) {
	fmt.Printf("%s: %s\n", label, formatSize(bytes))
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
