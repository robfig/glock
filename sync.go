package main

import (
	"bufio"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"
)

var cmdSync = &Command{
	UsageLine: "sync [import path]",
	Short:     "sync current GOPATH with GLOCKFILE in the import path's root.",
	Long: `sync checks the GOPATH for consistency with the given package's GLOCKFILE

For example:

	glock sync github.com/robfig/glock

It verifies that each entry in the GLOCKFILE is at the expected revision.
If a dependency is not at the expected revision, it is re-downloaded and synced.
`,
}

func init() {
	cmdSync.Run = runSync // break init loop
}

func runSync(cmd *Command, args []string) {
	if len(args) == 0 {
		cmdSync.Usage()
		return
	}
	var importPath = args[0]
	var repo, err = repoRootForImportPath(importPath)
	if err != nil {
		perror(err)
	}

	var gopath = filepath.SplitList(build.Default.GOPATH)[0]
	glockfile, err := os.Open(filepath.Join(gopath, "src", repo.root, "GLOCKFILE"))
	if err != nil {
		perror(err)
	}

	var scanner = bufio.NewScanner(glockfile)
	for scanner.Scan() {
		var fields = strings.Fields(scanner.Text())
		var importPath, expectedRevision = fields[0], fields[1]
		var repo, err = repoRootForImportPath(importPath)
		if err != nil {
			fmt.Println("error determining repo root for", importPath, err)
			continue
		}

		actualRevision, err := repo.vcs.head(filepath.Join(gopath, "src", repo.root), repo.repo)
		if err != nil {
			fmt.Println("error determining revision of", repo.root, err)
			continue
		}
		if expectedRevision == actualRevision {
			fmt.Println(importPath, "at", expectedRevision)
			continue
		}

		fmt.Println("updating", importPath, "from", actualRevision, "to", expectedRevision)
		var importDir = filepath.Join(gopath, "src", importPath)
		err = repo.vcs.download(importDir)
		if err != nil {
			fmt.Println("error downloading", importPath, "to", importDir, "-", err)
			continue
		}
		err = repo.vcs.tagSync(importDir, expectedRevision)
		if err != nil {
			fmt.Println("error syncing", importPath, "to", expectedRevision, "-", err)
			continue
		}
	}
}
