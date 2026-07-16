package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"mcsync/internal/dockerutil"
	"mcsync/internal/gitutil"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the server, then commit and push world/config/mods changes",
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

// trackedStatePaths are the parts of data/ that .gitignore keeps (see
// scaffold.Gitignore): world, config, server.properties, ops/whitelist,
// and mods (tracked via Git LFS -- see scaffold.ModsLFSPattern).
var trackedStatePaths = []string{
	filepath.Join("data", "world"),
	filepath.Join("data", "config"),
	filepath.Join("data", "server.properties"),
	filepath.Join("data", "ops.json"),
	filepath.Join("data", "whitelist.json"),
	filepath.Join("data", "mods"),
}

func runStop(c *cobra.Command, args []string) error {
	dir := "."

	fmt.Println("Stopping the server...")
	if err := dockerutil.Compose(dir, "down"); err != nil {
		return err
	}

	if !gitutil.IsInstalled() || !gitutil.IsRepo(dir) {
		fmt.Println("Not a git repo; skipping save. Run `mcsync init` first if you want world/config synced.")
		return nil
	}

	var toAdd []string
	for _, p := range trackedStatePaths {
		if _, err := os.Stat(filepath.Join(dir, p)); err == nil {
			toAdd = append(toAdd, p)
		}
	}
	if len(toAdd) == 0 {
		fmt.Println("No world/config/mods data found to save.")
		return nil
	}

	if err := gitutil.Run(dir, append([]string{"add"}, toAdd...)...); err != nil {
		return err
	}
	changed, err := gitutil.HasStagedChanges(dir)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("No world/config/mods changes since last save.")
		return nil
	}

	msg := fmt.Sprintf("Save world state: %s", time.Now().Format("2006-01-02 15:04:05"))
	if err := gitutil.Run(dir, "commit", "-m", msg); err != nil {
		return err
	}

	if gitutil.HasRemote(dir, "origin") {
		fmt.Println("Pushing...")
		if err := gitutil.Run(dir, "push"); err != nil {
			return fmt.Errorf("%w\n(the save was committed locally; push manually once this is resolved)", err)
		}
	} else {
		fmt.Println("No git remote configured; saved locally only.")
	}

	fmt.Println("\nDone. World/config saved.")
	return nil
}
