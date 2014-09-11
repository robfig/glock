package main

import (
	"bufio"
	"bytes"
	"flag"
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
Commands are built if necessary.
`,
}

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
	glockfile, err := os.Open(filepath.Join(gopath, "src", repo.root, "GLOCKFILE"))
	if err != nil {
		perror(err)
	}

	var cmds []string
	var scanner = bufio.NewScanner(glockfile)
	for scanner.Scan() {
		var fields = strings.Fields(scanner.Text())
		if fields[0] == "cmd" {
			cmds = append(cmds, fields[1])
			continue
		}

		var importPath, expectedRevision = fields[0], truncate(fields[1])
		var importDir = filepath.Join(gopath, "src", importPath)

		// Try to find the repo.
		// go get it beforehand just in case it doesn't exist. (no-op if it does exist)
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

		// If we didn't just get this package, download it now to update.
		if maybeGot == "" {
			err = repo.vcs.download(importDir)
			if err != nil {
				perror(err)
			}
		}

		// Checkout the expected revision.  Don't use tagSync because it runs "git show-ref"
		// which returns error if the revision does not correspond to a tag or head.
		err = repo.vcs.run(importDir, repo.vcs.tagSyncCmd, "tag", expectedRevision)
		if err != nil {
			perror(err)
		}
	}
	if scanner.Err() != nil {
		perror(scanner.Err())
	}

	// Install the commands.
	for _, cmd := range cmds {
		// any updated packages should have been cleaned by the previous step.
		// "go install" will do it. (aside from one pathological case, meh)
		fmt.Printf("cmd %-59.58s\t", cmd)
		installOutput, err := run("go", "install", "-v", cmd)
		switch {
		case err != nil:
			fmt.Println("[" + critical("error") + " " + err.Error() + "]")
		case len(bytes.TrimSpace(installOutput)) > 0:
			fmt.Println("[" + warning("built") + "]")
		default:
			fmt.Println("[" + info("OK") + "]")
		}
	}
}

// truncate a revision to the 12-digit prefix.
func truncate(rev string) string {
	rev = strings.TrimSpace(rev)
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}
