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
	Long:      `TODO`,
}

func init() {
	cmdApply.Run = runApply // break init loop
}

func runApply(cmd *Command, args []string) {
	var gopath = filepath.SplitList(build.Default.GOPATH)[0]
	var cmds = buildCommands(readDiffLines(os.Stdin))
	for _, cmd := range cmds {
		var importDir = path.Join(gopath, "src", cmd.importPath)
		switch cmd.action {
		case remove:
			fmt.Println(cmd.importPath, "is no longer in use.")
		case add, update:
			var repo, err = repoRootForImportPath(cmd.importPath)
			if err != nil {
				fmt.Println("error determining repo root for", cmd.importPath, err)
				continue
			}

			err = repo.vcs.download(importDir)
			if err != nil {
				fmt.Println("error downloading", cmd.importPath, "to", importDir, "-", err)
				continue
			}

			err = repo.vcs.tagSync(importDir, cmd.revision)
			if err != nil {
				fmt.Println("error syncing", cmd.importPath, "to", cmd.revision, "-", err)
				continue
			}
		}
	}
}
