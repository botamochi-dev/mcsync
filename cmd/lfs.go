package cmd

import (
	"fmt"

	"mcsync/internal/gitutil"
)

// pruneLFSCache runs `git lfs prune` in dir to keep .git/lfs/objects from
// growing unbounded as mod jars in data/mods are added or updated over
// time. It's a maintenance nicety, not core functionality, so failures are
// reported but never abort the calling command. It only runs when both
// git-lfs and a remote are available, since prune needs the remote to
// verify an object is safe to delete locally.
func pruneLFSCache(dir string) {
	if !gitutil.LFSInstalled() || !gitutil.HasRemote(dir, "origin") {
		return
	}
	fmt.Println("Cleaning up local Git LFS cache...")
	if err := gitutil.LFSPrune(dir); err != nil {
		fmt.Printf("Warning: `git lfs prune` failed: %v\n", err)
	}
}
