package main

import (
	"fmt"
	"go/build"
	"os"
	"path"
	"path/filepath"
)

var cmdApply = &Command{
	UsageLine: "apply",
	Short:     "apply the changes described by a GLOCKFILE diff (on STDIN) to the current GOPATH.",
	Long: `apply the changes described by a GLOCKFILE diff (on STDIN) to the current GOPATH.

It is meant to be called from a VCS hook on any change to the GLOCKFILE.
`,
}

func init() {
	cmdApply.Run = runApply // break init loop
}

var actionstr = map[action]string{
	add:    "add   ",
	update: "update",
	remove: "remove",
}

func runApply(cmd *Command, args []string) {
	var gopath = filepath.SplitList(build.Default.GOPATH)[0]
	var cmds = buildCommands(readDiffLines(os.Stdin))
	for _, cmd := range cmds {
		fmt.Printf("%s %-50.49s %s\n", actionstr[cmd.action], cmd.importPath, cmd.revision)
		var importDir = path.Join(gopath, "src", cmd.importPath)
		switch cmd.action {
		case remove:
			// do nothing
		case add, update:
			// add or update the dependency
			run("go", "get", "-u", "-d", path.Join(cmd.importPath, "..."))

			// update that dependency
			var repo, err = repoRootForImportPath(cmd.importPath)
			if err != nil {
				fmt.Println("error determining repo root for", cmd.importPath, err)
				continue
			}
			err = repo.vcs.run(importDir, repo.vcs.tagSyncCmd, "tag", cmd.revision)
			if err != nil {
				fmt.Println("error syncing", cmd.importPath, "to", cmd.revision, "-", err)
				continue
			}

			// clean existing package
			run("go", "clean", "-i", path.Join(cmd.importPath, "..."))
		}
	}
}
