package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"mcsync/internal/gitutil"
)

var autosaveInterval time.Duration

var autosaveCmd = &cobra.Command{
	Use:   "autosave",
	Short: "Periodically save (commit/push) world/config/mods while the server keeps running",
	Long: `Periodically save (commit/push) world/config/mods while the server keeps
running -- unlike "mcsync stop", this does not take the server down.

Runs in the foreground until interrupted (Ctrl+C). Before each save, it
best-effort runs "save-all" over RCON so the snapshot isn't taken mid-write.`,
	RunE: runAutosave,
}

func init() {
	autosaveCmd.Flags().DurationVar(&autosaveInterval, "interval", 15*time.Minute, "how often to save")
	rootCmd.AddCommand(autosaveCmd)
}

func runAutosave(c *cobra.Command, args []string) error {
	dir := "."
	if !gitutil.IsInstalled() || !gitutil.IsRepo(dir) {
		return fmt.Errorf("not a git repo; run `mcsync init` first")
	}
	fmt.Printf("Autosaving every %s. This does not stop the server -- press Ctrl+C to stop autosaving.\n", autosaveInterval)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	ticker := time.NewTicker(autosaveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-sigCh:
			fmt.Println("\nStopped autosaving (server keeps running).")
			return nil
		case <-ticker.C:
			fmt.Printf("\n[%s] Autosaving...\n", time.Now().Format("15:04:05"))
			if err := saveTrackedState(dir, true); err != nil {
				fmt.Printf("Autosave failed: %v\n", err)
			}
		}
	}
}
