package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"mcsync/internal/dockerutil"
	"mcsync/internal/gitutil"
)

var startNoWait bool

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
	rootCmd.AddCommand(startCmd)
}

func runStart(c *cobra.Command, args []string) error {
	dir := "."

	if gitutil.IsInstalled() && gitutil.IsRepo(dir) {
		if gitutil.HasRemote(dir, "origin") {
			fmt.Println("Pulling latest changes...")
			if err := gitutil.Run(dir, "pull", "--ff-only"); err != nil {
				return fmt.Errorf("%w\n(if this is a merge conflict, resolve it manually before starting the server -- "+
					"do not start with unresolved world/config conflicts)", err)
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
