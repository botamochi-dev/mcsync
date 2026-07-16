// Package cmd implements the mcsync CLI. mcsync is a thin wrapper around
// git and docker compose: docker-compose.yml is the manifest (MC/Forge
// version, mod list), itzg/docker-minecraft-server handles the Forge
// install/EULA/mod download, and mcsync only saves users from typing the
// same git+docker commands every time.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mcsync",
	Short: "Sync a Forge Minecraft server (world + config) across PCs via git + docker compose",
	Long: `mcsync wraps git and docker compose so a self-hosted Forge Minecraft
server's world/config can be synced across multiple PCs.

docker-compose.yml is the manifest: it declares the Minecraft/Forge version
and mod list. mcsync never downloads Forge or mods itself -- that's handled
by the itzg/minecraft-server image on container start.`,
	SilenceUsage: true,
}

// Execute runs the CLI, printing any error and returning it so main can
// set a non-zero exit code.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return err
	}
	return nil
}
