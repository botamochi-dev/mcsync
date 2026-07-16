package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"mcsync/internal/dockerutil"
	"mcsync/internal/gitutil"
)

var (
	startNoWait bool
	startForce  bool
)

// startupTimeout is generous because the first boot on a fresh PC downloads
// and installs Forge plus every mod from scratch.
const startupTimeout = 30 * time.Minute

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Pull the latest world/config from git, then start the server",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().BoolVar(&startNoWait, "no-wait", false, "return immediately instead of showing startup progress")
	startCmd.Flags().BoolVar(&startForce, "force", false, "start even if another PC's lock looks active")
	rootCmd.AddCommand(startCmd)
}

func runStart(c *cobra.Command, args []string) error {
	dir := "."
	tracked := gitutil.IsInstalled() && gitutil.IsRepo(dir)
	remoteConfigured := tracked && gitutil.HasRemote(dir, "origin")

	if tracked {
		if remoteConfigured {
			fmt.Println("Pulling latest changes...")
			if err := gitutil.Run(dir, "pull", "--ff-only"); err != nil {
				return fmt.Errorf("%w\n(if this is a merge conflict, resolve it manually before starting the server -- "+
					"do not start with unresolved world/config conflicts)", err)
			}
			pruneLFSCache(dir)
			if err := checkLock(dir, startForce); err != nil {
				return err
			}
		} else {
			fmt.Println("No git remote configured; skipping pull.")
		}
	}

	if !dockerutil.DaemonRunning() {
		return fmt.Errorf("Docker daemon isn't running; start Docker Desktop and try again")
	}

	fmt.Println("Starting the server...")
	if err := dockerutil.Compose(dir, "up", "-d"); err != nil {
		return err
	}

	if remoteConfigured {
		if err := writeLockAndCommit(dir); err != nil {
			fmt.Printf("Warning: couldn't record start lock: %v\n", err)
		}
	}

	if startNoWait {
		fmt.Println("\nServer starting in the background. Watch logs with: docker compose logs -f mc")
		return nil
	}

	fmt.Println("Waiting for the server to finish starting (first boot on a new PC can take a while -- " +
		"Forge and mods are downloaded from scratch)...")
	if err := dockerutil.WaitHealthy(dir, "mc", startupTimeout); err != nil {
		return err
	}
	fmt.Println("\nServer is up. Connect with localhost in Minecraft's multiplayer screen.")
	return nil
}
