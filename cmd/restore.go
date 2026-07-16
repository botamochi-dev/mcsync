package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"mcsync/internal/dockerutil"
	"mcsync/internal/gitutil"
	"mcsync/internal/prompt"
)

var restoreYes bool

var restoreCmd = &cobra.Command{
	Use:   "restore [commit]",
	Short: "Roll back world/config/mods to a previous save from git history",
	Long: `Roll back world/config/mods to a previous save from git history.

Every "mcsync stop" creates a commit, so git history doubles as a save
history. With no argument, lists recent saves to pick from. With a commit
hash, restores data/world, data/config, data/mods, server.properties,
ops.json, and whitelist.json to that commit's versions, as a new commit
(nothing is discarded -- the current state stays reachable in history).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRestore,
}

func init() {
	restoreCmd.Flags().BoolVarP(&restoreYes, "yes", "y", false, "skip the confirmation prompt")
	rootCmd.AddCommand(restoreCmd)
}

func runRestore(c *cobra.Command, args []string) error {
	dir := "."
	if !gitutil.IsInstalled() || !gitutil.IsRepo(dir) {
		return fmt.Errorf("not a git repo; nothing to restore from")
	}

	if len(args) == 0 {
		return listRestorePoints(dir)
	}
	commit := args[0]

	if _, err := gitutil.Output(dir, "rev-parse", "--verify", commit+"^{commit}"); err != nil {
		return fmt.Errorf("%q doesn't look like a valid commit (run `mcsync restore` with no argument to list recent saves)", commit)
	}

	if state, _, err := dockerutil.ServiceStatus(dir, "mc"); err == nil && state == "running" {
		return fmt.Errorf("the server is currently running; run `mcsync stop` first so restored files aren't immediately overwritten")
	}

	hash, date, subject, err := gitutil.CommitInfo(dir, commit)
	if err != nil {
		return err
	}

	// Only restore paths that actually existed at that commit --
	// `git checkout <rev> -- <path>` errors on any pathspec absent there
	// (e.g. an early save made before data/mods was first tracked).
	var paths []string
	for _, p := range trackedStatePaths {
		if gitutil.PathExistsAtRevision(dir, commit, p) {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return fmt.Errorf("none of the tracked paths (%s) existed yet at %s", strings.Join(trackedStatePaths, ", "), hash)
	}

	fmt.Printf("This will overwrite the CURRENT %s\nwith the versions from:\n\n  %s  %s\n  %s\n\n"+
		"Your current state isn't lost (it stays in git history) but will no longer be what's on disk.\n\n",
		strings.Join(paths, ", "), hash, date, subject)
	if !restoreYes && !prompt.Confirm("Continue?", false) {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := gitutil.Run(dir, append([]string{"checkout", commit, "--"}, paths...)...); err != nil {
		return err
	}
	changed, err := gitutil.HasStagedChanges(dir)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("Already at that state; nothing to restore.")
		return nil
	}

	msg := fmt.Sprintf("Restore world state from %s (%s)", hash, date)
	if err := gitutil.Run(dir, "commit", "-m", msg); err != nil {
		return err
	}

	if gitutil.HasRemote(dir, "origin") {
		fmt.Println("Pushing...")
		if err := gitutil.Run(dir, "push"); err != nil {
			return fmt.Errorf("%w\n(the restore was committed locally; push manually once this is resolved)", err)
		}
		pruneLFSCache(dir)
	}

	fmt.Println("\nDone. Run `mcsync start` to launch the server with the restored state.")
	return nil
}

func listRestorePoints(dir string) error {
	entries, err := gitutil.LogPaths(dir, 20, "data/world")
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No saves found yet (data/world has no history).")
		return nil
	}
	fmt.Println("Recent saves (commits touching data/world):")
	for _, e := range entries {
		fmt.Printf("  %s  %s  %s\n", e.Hash, e.Date, e.Subject)
	}
	fmt.Println("\nRun `mcsync restore <hash>` to roll back to one of these.")
	return nil
}
