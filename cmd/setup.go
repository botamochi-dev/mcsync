package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"mcsync/internal/dockerutil"
	"mcsync/internal/gitutil"
)

var setupCmd = &cobra.Command{
	Use:   "setup <git-url> [dir]",
	Short: "Clone an existing mcsync project and start the server (resume on a new PC)",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(c *cobra.Command, args []string) error {
	repoURL := args[0]
	dir := ""
	if len(args) == 2 {
		dir = args[1]
	} else {
		dir = dirNameFromRepoURL(repoURL)
	}

	if !gitutil.IsInstalled() {
		return fmt.Errorf("git is not installed; run `mcsync doctor` for details")
	}
	if !gitutil.LFSInstalled() {
		return fmt.Errorf("git-lfs is not installed; mcsync projects track mod jars (data/mods) with it, " +
			"and without it they'd silently clone as empty placeholder files. " +
			"Install it from https://git-lfs.com and try again")
	}
	if !dockerutil.IsInstalled() {
		return fmt.Errorf("docker is not installed; run `mcsync doctor` for details")
	}
	if !dockerutil.DaemonRunning() {
		return fmt.Errorf("Docker daemon isn't running; start Docker Desktop and try again")
	}
	if !dockerutil.ComposeInstalled() {
		return fmt.Errorf("`docker compose` plugin isn't available; run `mcsync doctor` for details")
	}

	// Make sure git-lfs's filters are registered even if `git lfs install`
	// was never run on this PC before -- otherwise the clone below would
	// silently leave mod jars as tiny LFS pointer files instead of the
	// real content.
	if err := gitutil.LFSInstall("."); err != nil {
		return fmt.Errorf("`git lfs install` failed: %w", err)
	}

	fmt.Printf("Cloning %s into %s...\n", repoURL, dir)
	if err := gitutil.Run(".", "clone", repoURL, dir); err != nil {
		return err
	}

	fmt.Println("Starting the server...")
	if err := dockerutil.Compose(dir, "up", "-d"); err != nil {
		return err
	}

	fmt.Println("Waiting for the server to finish starting (first boot on a new PC can take a while -- " +
		"Forge and mods are downloaded from scratch)...")
	if err := dockerutil.WaitHealthy(dir, "mc", startupTimeout); err != nil {
		return err
	}
	fmt.Println("\nServer is up. Connect with localhost in Minecraft's multiplayer screen.")
	return nil
}

func dirNameFromRepoURL(repoURL string) string {
	name := repoURL
	if i := strings.LastIndexAny(name, "/\\"); i != -1 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".git")
	if name == "" {
		name = "mcsync-server"
	}
	return filepath.Clean(name)
}
