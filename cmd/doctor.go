package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mcsync/internal/gitutil"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that git and git-lfs are installed and ready",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

type check struct {
	label string
	ok    bool
	hint  string
}

func runDoctor(c *cobra.Command, args []string) error {
	checks := []check{
		{"git installed", gitutil.IsInstalled(), "install git: https://git-scm.com/downloads"},
	}
	if checks[0].ok {
		checks = append(checks,
			check{"git-lfs installed", gitutil.LFSInstalled(), "install git-lfs (used to sync mod jars in data/mods): https://git-lfs.com"},
		)
	}

	allOK := true
	for _, ch := range checks {
		mark := "✅"
		if !ch.ok {
			mark = "❌"
			allOK = false
		}
		fmt.Printf("%s %s\n", mark, ch.label)
		if !ch.ok && ch.hint != "" {
			fmt.Printf("   -> %s\n", ch.hint)
		}
	}

	fmt.Println("\nNote: Java and Forge aren't checked here -- mcsync downloads and manages both itself " +
		"(cached under your user profile) the first time you run `mcsync start`/`setup`.")

	if !allOK {
		return fmt.Errorf("one or more checks failed")
	}
	fmt.Println("All good.")
	return nil
}
