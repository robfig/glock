package main

import (
	"bufio"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"

	"github.com/agtorre/gocolorize"
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
	var repo, err = glockRepoRootForImportPath(importPath)
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
		var importDir = filepath.Join(gopath, "src", importPath)

		// Try to find the repo.  If it doesn't exist, get it.
		var repo, err = glockRepoRootForImportPath(importPath)
		if err != nil {
			run("go", "get", importPath)
			repo, err = glockRepoRootForImportPath(importPath)
		}
		if err != nil {
			perror(err)
		}

		actualRevision, err := repo.vcs.head(filepath.Join(gopath, "src", repo.root), repo.repo)
		if err != nil {
			fmt.Println("error determining revision of", repo.root, err)
			continue
		}
		actualRevision = strings.TrimSpace(actualRevision)
		fmt.Printf("%-50.49s %-12.12s\t", importPath, actualRevision)
		if expectedRevision == actualRevision {
			fmt.Print("[", info("OK"), "]\n")
			continue
		}

		fmt.Println("[" + warning(fmt.Sprintf("checkout %-12.12s", expectedRevision)) + "]")
		err = repo.vcs.download(importDir)
		if err != nil {
			perror(err)
		}

		err = repo.vcs.tagSync(importDir, expectedRevision)
		if err != nil {
			perror(err)
		}
	}
}

var (
	info     = gocolorize.NewColor("green").Paint
	warning  = gocolorize.NewColor("yellow").Paint
	critical = gocolorize.NewColor("red").Paint
)
