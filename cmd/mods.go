package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"mcsync/internal/composeedit"
	"mcsync/internal/dockerutil"
	"mcsync/internal/gitutil"
	"mcsync/internal/modurl"
)

var (
	modsAddType    string
	modsAddApply   bool
	modsRemoveType string
)

var modsCmd = &cobra.Command{
	Use:   "mods",
	Short: "Manage the mod list in docker-compose.yml",
}

var modsAddCmd = &cobra.Command{
	Use:   "add <modrinth-or-curseforge-url-or-slug>",
	Short: "Add a mod to docker-compose.yml (Modrinth or CurseForge)",
	Args:  cobra.ExactArgs(1),
	RunE:  runModsAdd,
}

var modsRemoveCmd = &cobra.Command{
	Use:   "remove <modrinth-or-curseforge-url-or-slug>",
	Short: "Remove a mod from docker-compose.yml",
	Args:  cobra.ExactArgs(1),
	RunE:  runModsRemove,
}

var modsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List mods declared in docker-compose.yml and jar files present in data/mods",
	Args:  cobra.NoArgs,
	RunE:  runModsList,
}

func init() {
	modsAddCmd.Flags().StringVar(&modsAddType, "type", "modrinth", "platform to use when a bare slug (not a URL) is given: modrinth or curseforge")
	modsAddCmd.Flags().BoolVar(&modsAddApply, "apply", false, "run `docker compose up -d` immediately to apply the change")
	modsRemoveCmd.Flags().StringVar(&modsRemoveType, "type", "modrinth", "platform to use when a bare slug (not a URL) is given: modrinth or curseforge")
	modsCmd.AddCommand(modsAddCmd, modsRemoveCmd, modsListCmd)
	rootCmd.AddCommand(modsCmd)
}

const composeFile = "docker-compose.yml"

func runModsAdd(c *cobra.Command, args []string) error {
	fallback := modurl.Modrinth
	if modsAddType == "curseforge" {
		fallback = modurl.CurseForge
	}
	platform, slug, err := modurl.Parse(args[0], fallback)
	if err != nil {
		return err
	}

	doc, err := composeedit.Load(composeFile)
	if err != nil {
		return fmt.Errorf("%w (run this from the project root, next to docker-compose.yml)", err)
	}

	var added bool
	switch platform {
	case modurl.Modrinth:
		added, err = doc.AddModrinthProject(slug)
	case modurl.CurseForge:
		added, err = doc.AddCurseforgeFile(slug)
		if err == nil {
			fmt.Println("Note: CURSEFORGE_FILES requires a CF_API_KEY environment variable on the host " +
				"(CurseForge's API requires a key). See itzg/docker-minecraft-server docs.")
		}
	}
	if err != nil {
		return err
	}
	if !added {
		fmt.Printf("%s (%s) is already in docker-compose.yml; nothing to do.\n", slug, platform)
		return nil
	}
	if err := doc.Save(); err != nil {
		return err
	}
	fmt.Printf("Added %s mod: %s\n", platform, slug)

	if err := commitAndPushComposeFile(fmt.Sprintf("Add mod: %s (%s)", slug, platform)); err != nil {
		return err
	}

	if modsAddApply {
		fmt.Println("Applying (docker compose up -d)...")
		if err := dockerutil.Compose(".", "up", "-d"); err != nil {
			return err
		}
	} else {
		fmt.Println("Run `mcsync start` (or `docker compose up -d`) to apply this change.")
	}
	return nil
}

func runModsRemove(c *cobra.Command, args []string) error {
	fallback := modurl.Modrinth
	if modsRemoveType == "curseforge" {
		fallback = modurl.CurseForge
	}
	platform, slug, err := modurl.Parse(args[0], fallback)
	if err != nil {
		return err
	}

	doc, err := composeedit.Load(composeFile)
	if err != nil {
		return fmt.Errorf("%w (run this from the project root, next to docker-compose.yml)", err)
	}

	var removed bool
	switch platform {
	case modurl.Modrinth:
		removed, err = doc.RemoveModrinthProject(slug)
	case modurl.CurseForge:
		removed, err = doc.RemoveCurseforgeFile(slug)
	}
	if err != nil {
		return err
	}
	if !removed {
		fmt.Printf("%s (%s) isn't in docker-compose.yml; nothing to do.\n", slug, platform)
		return nil
	}
	if err := doc.Save(); err != nil {
		return err
	}
	fmt.Printf("Removed %s mod: %s\n", platform, slug)
	fmt.Println("Note: this doesn't delete an already-downloaded jar from data/mods -- " +
		"itzg/minecraft-server only removes old mod files itself if REMOVE_OLD_MODS is set. " +
		"Delete the jar from data/mods by hand if you want it fully gone.")

	if err := commitAndPushComposeFile(fmt.Sprintf("Remove mod: %s (%s)", slug, platform)); err != nil {
		return err
	}
	fmt.Println("Run `mcsync start` (or `docker compose up -d`) to apply this change.")
	return nil
}

func runModsList(c *cobra.Command, args []string) error {
	doc, err := composeedit.Load(composeFile)
	if err != nil {
		return fmt.Errorf("%w (run this from the project root, next to docker-compose.yml)", err)
	}
	modrinth, err := doc.ListModrinthProjects()
	if err != nil {
		return err
	}
	curseforge, err := doc.ListCurseforgeFiles()
	if err != nil {
		return err
	}

	fmt.Println("Modrinth (docker-compose.yml MODRINTH_PROJECTS):")
	printModList(modrinth)
	fmt.Println("\nCurseForge (docker-compose.yml CURSEFORGE_FILES):")
	printModList(curseforge)

	fmt.Println("\ndata/mods (jar files on disk):")
	entries, err := os.ReadDir(filepath.Join(".", "data", "mods"))
	if err != nil {
		fmt.Println("  (none -- server hasn't started yet, or data/mods doesn't exist)")
		return nil
	}
	var jars []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".jar") {
			jars = append(jars, e.Name())
		}
	}
	sort.Strings(jars)
	printModList(jars)
	return nil
}

func printModList(items []string) {
	if len(items) == 0 {
		fmt.Println("  (none)")
		return
	}
	for _, item := range items {
		fmt.Printf("  %s\n", item)
	}
}

// commitAndPushComposeFile stages docker-compose.yml, commits with msg,
// and pushes if a remote is configured. Used by both `mods add` and
// `mods remove` since a manifest change should be synced immediately
// rather than waiting for the next `stop`.
func commitAndPushComposeFile(msg string) error {
	if !gitutil.IsInstalled() || !gitutil.IsRepo(".") {
		return nil
	}
	if err := gitutil.Run(".", "add", composeFile); err != nil {
		return err
	}
	changed, err := gitutil.HasStagedChanges(".")
	if err != nil || !changed {
		return err
	}
	if err := gitutil.Run(".", "commit", "-m", msg); err != nil {
		return err
	}
	if gitutil.HasRemote(".", "origin") {
		if err := gitutil.Run(".", "push"); err != nil {
			return fmt.Errorf("%w\n(the change was committed locally; push manually once this is resolved)", err)
		}
	}
	return nil
}
