package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
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

Options:

	-n	read GLOCKFILE from stdin

`,
}

var (
	syncColor = cmdSync.Flag.Bool("color", true, "if true, colorize terminal output")
	syncN     = cmdSync.Flag.Bool("n", false, "Read GLOCKFILE from stdin")

	info     = gocolorize.NewColor("green").Paint
	warning  = gocolorize.NewColor("yellow").Paint
	critical = gocolorize.NewColor("red").Paint

	disabled = func(args ...interface{}) string { return fmt.Sprint(args...) }
)

// Running too many syncs at once can exhaust file descriptor limits.
// Empirically, ~90 is enough to hit the macOS default limit of 256.
const maxConcurrentSyncs = 25

func init() {
	cmdSync.Run = runSync // break init loop
}

func runSync(cmd *Command, args []string) {
	if len(args) == 0 && !*syncN {
		cmdSync.Usage()
		return
	}

	var importPath string
	if len(args) > 0 {
		importPath = args[0]
	}
	var glockfile = glockfileReader(importPath, *syncN)
	defer glockfile.Close()

	if !*syncColor {
		info = disabled
		warning = disabled
		critical = disabled
	}

	type pkgSpec struct {
		importPath, expectedRevision string
	}
	var pkgSpecs []pkgSpec
	var cmds []string
	var scanner = bufio.NewScanner(glockfile)
	for scanner.Scan() {
		var fields = strings.Fields(scanner.Text())
		if fields[0] == "cmd" {
			cmds = append(cmds, fields[1])
			continue
		}
		var importPath, expectedRevision = fields[0], truncate(fields[1])
		pkgSpecs = append(pkgSpecs, pkgSpec{importPath, expectedRevision})
	}
	if scanner.Err() != nil {
		perror(scanner.Err())
	}

	getArgs := []string{"get", "-v", "-d"}
	for _, pkgSpec := range pkgSpecs {
		getArgs = append(getArgs, pkgSpec.importPath)
	}

	// `go get` all the packages at once (rather than in parallel) since it is
	// unsafe to invoke `go get` concurrently. `go get` is a no-op if packages
	// are already present.
	//
	// See https://groups.google.com/forum/#!topic/golang-nuts/2BquR7EpMJs
	// which suggests using golang.org/x/tools/go/vcs instead of shelling out
	// to `go get`; unfortunately, it is not possible to sync to a specific
	// ref using the API exposed by that package at this time.
	getOutput, getErr := run("go", getArgs...)

	// Semaphore to limit concurrent sync operations
	var sem = make(chan struct{}, maxConcurrentSyncs)
	var chans []chan string
	for _, pkgSpec := range pkgSpecs {
		var ch = make(chan string, 1)
		chans = append(chans, ch)

		pkgSpec := pkgSpec

		go func() {
			sem <- struct{}{}
			syncPkg(ch, pkgSpec.importPath, pkgSpec.expectedRevision, string(getOutput), getErr)
			<-sem
		}()
	}

	for _, ch := range chans {
		fmt.Print(<-ch)
	}

	// Install the commands.
	for _, cmd := range cmds {
		// any updated packages should have been cleaned by the previous step.
		// "go install" will do it. (aside from one pathological case, meh)
		fmt.Printf("cmd %-59.58s\t", cmd)
		rawOutput, err := run("go", "install", "-v", cmd)
		output := string(bytes.TrimSpace(rawOutput))
		switch {
		case err != nil:
			fmt.Println("[" + critical("error") + " " + err.Error() + "]")
			perror(errors.New(output))
		case 0 < len(output):
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

func syncPkg(ch chan<- string, importPath, expectedRevision, getOutput string, getErr error) {
	var importDir = filepath.Join(gopaths()[0], "src", importPath)
	var status bytes.Buffer
	defer func() { ch <- status.String() }()

	// Try to find the repo.
	var repo, err = fastRepoRoot(importPath)
	if err != nil {
		var getStatus = "(success)"
		if getErr != nil {
			getStatus = string(getOutput) + getErr.Error()
		}
		perror(fmt.Errorf(`failed to get: %s

> go get -v -d %s
%s

> import %s
%s`, importPath, importPath, getStatus, importPath, err))
	}

	var maybeGot = ""
	if strings.Contains(getOutput, importPath+" (download)") {
		maybeGot = warning("get ")
	}

	actualRevision, err := repo.vcs.head(repo.path, repo.repo)
	if err != nil {
		fmt.Fprintln(&status, "error determining revision of", repo.root, err)
		perror(err)
	}

	actualRevision = truncate(actualRevision)
	fmt.Fprintf(&status, "%-50.49s %-12.12s\t", importPath, actualRevision)
	if expectedRevision == actualRevision {
		fmt.Fprint(&status, "[", maybeGot, info("OK"), "]\n")
		return
	}

	fmt.Fprintln(&status, "["+maybeGot+warning(fmt.Sprintf("checkout %-12.12s", expectedRevision))+"]")

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
