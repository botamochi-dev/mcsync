package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mcsync/internal/dockerutil"
)

var consoleCmd = &cobra.Command{
	Use:   "console [command...]",
	Short: "Open the server's RCON console, or run a single server command",
	Long: `Open the server's RCON console, or run a single server command.

With no arguments, opens an interactive console (equivalent to
"docker compose exec -it mc rcon-cli"). With arguments, runs that one
command and exits, e.g.:

  mcsync console list
  mcsync console op SomePlayer
  mcsync console save-all`,
	RunE: runConsole,
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}

func runConsole(c *cobra.Command, args []string) error {
	dir := "."
	if !dockerutil.DaemonRunning() {
		return fmt.Errorf("Docker daemon isn't running; start Docker Desktop and try again")
	}
	if _, _, err := dockerutil.ServiceStatus(dir, "mc"); err != nil {
		return fmt.Errorf("server isn't running; run `mcsync start` first")
	}
	if len(args) == 0 {
		return dockerutil.Compose(dir, "exec", "-it", "mc", "rcon-cli")
	}
	return dockerutil.Compose(dir, append([]string{"exec", "mc", "rcon-cli"}, args...)...)
}
