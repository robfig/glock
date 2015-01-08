package main

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agtorre/gocolorize"
)

var cmdSync = &Command{
	UsageLine: "sync [-v] [import path]",
	Short:     "sync current GOPATH with GLOCKFILE in the import path's root.",
	Long: `sync checks the GOPATH for consistency with the given package's GLOCKFILE

For example:

	glock sync github.com/robfig/glock

It verifies that each entry in the GLOCKFILE is at the expected revision.
If a dependency is not at the expected revision, it is re-downloaded and synced.
Commands are built if necessary.

Options:

        -v	print verbose output
	-n	read GLOCKFILE from stdin

`,
}

var (
	syncColor = cmdSync.Flag.Bool("color", true, "if true, colorize terminal output")
	verbose   = cmdSync.Flag.Bool("v", false, "if true, print verbose output")
	syncN     = cmdSync.Flag.Bool("n", false, "Read GLOCKFILE from stdin")

	info     = gocolorize.NewColor("green").Paint
	warning  = gocolorize.NewColor("yellow").Paint
	critical = gocolorize.NewColor("red").Paint

	disabled = func(args ...interface{}) string { return fmt.Sprint(args...) }
)

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

	var chans []chan string
	var cmds []string
	var scanner = bufio.NewScanner(glockfile)
	for scanner.Scan() {
		var fields = strings.Fields(scanner.Text())
		if fields[0] == "cmd" {
			cmds = append(cmds, fields[1])
			continue
		}
		var importPath, expectedRevision = fields[0], truncate(fields[1])
		var ch = make(chan string)
		chans = append(chans, ch)
		go syncPkg(ch, importPath, expectedRevision)
	}
	if scanner.Err() != nil {
		perror(scanner.Err())
	}

	for _, ch := range chans {
		fmt.Print(<-ch)
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
		if *verbose {
			trimmed := bytes.TrimSpace(installOutput)
			if 0 < len(trimmed) {
				fmt.Println(string(trimmed))
			}
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

func syncPkg(ch chan<- string, importPath, expectedRevision string) {
	var importDir = filepath.Join(gopath(), "src", importPath)
	var status bytes.Buffer
	defer func() { ch <- status.String() }()

	// Try to find the repo.
	var getOutput []byte
	var repo, err = fastRepoRoot(importPath)
	if err != nil {
		// go get it in case it doesn't exist. (no-op if it does exist)
		// (ignore failures due to "no buildable files" or build errors in the package.)
		getOutput, _ = run("go", "get", "-v", "-d", importPath)
		repo, err = fastRepoRoot(importPath)
		if err != nil {
			perror(err)
		}
	}

	var maybeGot = ""
	if bytes.Contains(getOutput, []byte("(download)")) {
		maybeGot = warning("get ")
	}

	actualRevision, err := repo.vcs.head(filepath.Join(gopath(), "src", repo.root), repo.repo)
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
