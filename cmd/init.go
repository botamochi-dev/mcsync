package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"mcsync/internal/forgeversion"
	"mcsync/internal/gitutil"
	"mcsync/internal/manifest"
	"mcsync/internal/prompt"
	"mcsync/internal/scaffold"
)

var (
	initName         string
	initMCVersion    string
	initForgeVersion string
	initMemory       string
	initRemote       string
	initDir          string
	initForce        bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new mcsync project (mcsync.yml + .gitignore + git repo)",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVar(&initName, "name", "", "project name")
	initCmd.Flags().StringVar(&initMCVersion, "mc-version", "", "Minecraft version, e.g. 1.20.1")
	initCmd.Flags().StringVar(&initForgeVersion, "forge-version", "", "Forge version; leave empty to auto-pick the recommended build")
	initCmd.Flags().StringVar(&initMemory, "memory", "", "memory allocated to the server, e.g. 4G")
	initCmd.Flags().StringVar(&initRemote, "remote", "", "git remote URL to push the initial commit to (optional)")
	initCmd.Flags().StringVar(&initDir, "dir", ".", "directory to create the project in")
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite an existing mcsync.yml")
	rootCmd.AddCommand(initCmd)
}

func runInit(c *cobra.Command, args []string) error {
	dir, err := filepath.Abs(initDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	manifestPath := filepath.Join(dir, manifest.FileName)
	if _, err := os.Stat(manifestPath); err == nil && !initForce {
		return fmt.Errorf("%s already exists (use --force to overwrite)", manifestPath)
	}
	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil && !initForce {
		return fmt.Errorf("%s already exists (use --force to overwrite)", gitignorePath)
	}

	if initName == "" {
		initName = prompt.Ask("Project name", filepath.Base(dir))
	}
	if initMCVersion == "" {
		initMCVersion = prompt.Ask("Minecraft version", "1.20.1")
	}
	if initForgeVersion == "" {
		initForgeVersion = prompt.Ask("Forge version (blank = auto-pick recommended)", "")
	}
	if initForgeVersion == "" {
		fmt.Printf("Looking up recommended Forge version for Minecraft %s...\n", initMCVersion)
		v, err := forgeversion.Recommended(initMCVersion)
		if err != nil {
			return fmt.Errorf("auto-picking Forge version: %w (pass --forge-version to set it manually)", err)
		}
		initForgeVersion = v
		fmt.Printf("Using Forge %s\n", initForgeVersion)
	}
	if initMemory == "" {
		initMemory = prompt.Ask("Memory", "4G")
	}
	if initRemote == "" {
		initRemote = prompt.Ask("Git remote URL (blank = skip)", "")
	}

	projectName := scaffold.SanitizeProjectName(initName)
	manifestContent, err := scaffold.RenderManifest(scaffold.ManifestData{
		ProjectName:  projectName,
		MCVersion:    initMCVersion,
		ForgeVersion: initForgeVersion,
		Memory:       initMemory,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", manifestPath, err)
	}
	if err := os.WriteFile(gitignorePath, []byte(scaffold.Gitignore), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", gitignorePath, err)
	}
	fmt.Printf("Wrote %s and %s\n", manifestPath, gitignorePath)

	if !gitutil.IsInstalled() {
		fmt.Println("git is not installed; skipping repo setup. Run `mcsync doctor` for details.")
		return nil
	}

	if !gitutil.IsRepo(dir) {
		if err := gitutil.Run(dir, "init", "-b", "main"); err != nil {
			return err
		}
	}

	if gitutil.LFSInstalled() {
		if err := gitutil.LFSInstall(dir); err != nil {
			fmt.Printf("Warning: `git lfs install` failed: %v\n", err)
		} else if err := gitutil.LFSTrack(dir, scaffold.ModsLFSPattern); err != nil {
			fmt.Printf("Warning: `git lfs track` failed: %v\n", err)
		} else {
			fmt.Printf("Mod jars under data/mods will be tracked via Git LFS (%s)\n", scaffold.ModsLFSPattern)
		}
	} else {
		fmt.Printf("Note: git-lfs isn't installed, so mod jars placed in data/mods will be tracked as regular "+
			"(non-LFS) git objects. This works but can bloat the repo for large/frequently-updated jars. "+
			"Install git-lfs (https://git-lfs.com) and run `git lfs track \"%s\"` later to switch.\n", scaffold.ModsLFSPattern)
	}

	addArgs := []string{"add", manifest.FileName, ".gitignore"}
	if _, err := os.Stat(filepath.Join(dir, ".gitattributes")); err == nil {
		addArgs = append(addArgs, ".gitattributes")
	}
	if err := gitutil.Run(dir, addArgs...); err != nil {
		return err
	}
	changed, err := gitutil.HasStagedChanges(dir)
	if err != nil {
		return err
	}
	if changed {
		if err := gitutil.Run(dir, "commit", "-m", "Initial commit: mcsync project setup"); err != nil {
			return err
		}
	} else {
		fmt.Println("Nothing new to commit.")
	}

	if initRemote != "" {
		if !gitutil.HasRemote(dir, "origin") {
			if err := gitutil.Run(dir, "remote", "add", "origin", initRemote); err != nil {
				return err
			}
		}
		branch := gitutil.CurrentBranch(dir)
		if branch == "" {
			branch = "main"
		}
		if err := gitutil.Run(dir, "push", "-u", "origin", branch); err != nil {
			return fmt.Errorf("pushing to %s: %w (you can push manually later)", initRemote, err)
		}
	}

	fmt.Println("\nDone. Next: `mcsync start` to launch the server " +
		"(first start downloads Java and Forge automatically -- can take a few minutes).")
	return nil
}
