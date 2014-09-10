package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/agtorre/gocolorize"
)

var cmdSync = &Command{
	UsageLine: "sync [import path]",
	Short:     "sync current GOPATH with GLOCKFILE in the import path's root.",
	Long: `sync checks the GOPATH for consistency with the given package's GLOCKFILE

Options:

	-n	read from stdin instead of reading from a file.

For example:

	glock sync github.com/robfig/glock

It verifies that each entry in the GLOCKFILE is at the expected revision.
If a dependency is not at the expected revision, it is re-downloaded and synced.
`,
}

var readN = cmdSync.Flag.Bool("n", false, "Don't load the GLOCKFILE, just read from stdin")

var (
	color = flag.Bool("color", true, "if true, colorize terminal output")

	info     = gocolorize.NewColor("green").Paint
	warning  = gocolorize.NewColor("yellow").Paint
	critical = gocolorize.NewColor("red").Paint

	disabled = func(args ...interface{}) string { return fmt.Sprint(args...) }
)

func init() {
	cmdSync.Run = runSync // break init loop
}

func runSync(cmd *Command, args []string) {
	if len(args) == 0 {
		cmdSync.Usage()
		return
	}
	if !*color {
		info = disabled
		warning = disabled
		critical = disabled
	}
	var importPath = args[0]
	var repo, err = glockRepoRootForImportPath(importPath)
	if err != nil {
		perror(err)
	}

	var gopath = filepath.SplitList(build.Default.GOPATH)[0]
	glockfile, err := readGlockFile(gopath, repo.root)
	if err != nil {
		perror(err)
	}
	var scanner = bufio.NewScanner(bytes.NewReader(glockfile))
	for scanner.Scan() {
		var fields = strings.Fields(scanner.Text())
		var importPath, expectedRevision = fields[0], truncate(fields[1])
		var importDir = filepath.Join(gopath, "src", importPath)

		// Try to find the repo.
		// go get it before hand just in case it doesn't exist. (no-op if it does exist)
		// (ignore failures due to "no buildable files" or build errors in the package.)
		var getOutput, _ = run("go", "get", "-v", "-d", importPath)
		var repo, err = glockRepoRootForImportPath(importPath)
		if err != nil {
			fmt.Println(string(getOutput)) // in case the get failed due to connection error
			perror(err)
		}

		var maybeGot = ""
		if bytes.Contains(getOutput, []byte("(download)")) {
			maybeGot = warning("get ")
		}

		actualRevision, err := repo.vcs.head(filepath.Join(gopath, "src", repo.root), repo.repo)
		if err != nil {
			fmt.Println("error determining revision of", repo.root, err)
			continue
		}
		actualRevision = truncate(actualRevision)
		fmt.Printf("%-50.49s %-12.12s\t", importPath, actualRevision)
		if expectedRevision == actualRevision {
			fmt.Print("[", maybeGot, info("OK"), "]\n")
			continue
		}

		fmt.Println("[" + maybeGot + warning(fmt.Sprintf("checkout %-12.12s", expectedRevision)) + "]")
		err = repo.vcs.download(importDir)
		if err != nil {
			perror(err)
		}

		// Checkout the expected revision.  Don't use tagSync because it runs "git show-ref"
		// which returns error if the revision does not correspond to a tag or head.
		err = repo.vcs.run(importDir, repo.vcs.tagSyncCmd, "tag", expectedRevision)
		if err != nil {
			perror(err)
		}

		// Clean the Go package to force rebuild.
		_, err = run("go", "clean", "-i", path.Join(importPath, "..."))
		if err != nil {
			fmt.Println("failed to clean:", err)
		}
	}

	if scanner.Err() != nil {
		perror(scanner.Err())
	}
}
func readGlockFile(gopath, repoRoot string) ([]byte, error) {
	if *readN {
		return ioutil.ReadAll(os.Stdin)
	}

	glockfile, err := os.Open(filepath.Join(gopath, "src", repoRoot, "GLOCKFILE"))
	if err != nil {
		return nil, err
	}
	defer glockfile.Close()
	return ioutil.ReadAll(glockfile)
}

// truncate a revision to the 12-digit prefix.
func truncate(rev string) string {
	rev = strings.TrimSpace(rev)
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}
