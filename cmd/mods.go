package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mcsync/internal/composeedit"
	"mcsync/internal/dockerutil"
	"mcsync/internal/gitutil"
	"mcsync/internal/modurl"
)

var (
	modsAddType  string
	modsAddApply bool
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

func init() {
	modsAddCmd.Flags().StringVar(&modsAddType, "type", "modrinth", "platform to use when a bare slug (not a URL) is given: modrinth or curseforge")
	modsAddCmd.Flags().BoolVar(&modsAddApply, "apply", false, "run `docker compose up -d` immediately to apply the change")
	modsCmd.AddCommand(modsAddCmd)
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

	if gitutil.IsInstalled() && gitutil.IsRepo(".") {
		if err := gitutil.Run(".", "add", composeFile); err != nil {
			return err
		}
		if err := gitutil.Run(".", "commit", "-m", fmt.Sprintf("Add mod: %s (%s)", slug, platform)); err != nil {
			return err
		}
		if gitutil.HasRemote(".", "origin") {
			if err := gitutil.Run(".", "push"); err != nil {
				return fmt.Errorf("%w\n(the change was committed locally; push manually once this is resolved)", err)
			}
		}
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
