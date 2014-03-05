package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/build"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

var cmdSave = &Command{
	UsageLine: "save [package]",
	Short:     "save a GLOCKFILE for the given package's dependencies",
	Long:      `TODO`,
}

var getN = cmdSave.Flag.Bool("n", false, "Don't save the file, just print to stdout")
var buildV bool

func init() {
	cmdSave.Run = runSave // break init loop
	cmdSave.Flag.BoolVar(&buildV, "v", false, "Verbose")
}

func runSave(cmd *Command, args []string) {
	// TODO: Ensure dependencies of tests are also included
	var output, err = exec.Command("go", "list", "-f", `{{range .Deps}}{{.}}{{"\n"}}{{end}}`, args[0]).
		CombinedOutput()
	if err != nil {
		perror(err)
	}

	var scanner = bufio.NewScanner(bytes.NewReader(output))
	var deps = map[string]struct{}{}
	for scanner.Scan() {
		var (
			pkg    = scanner.Text()
			slash  = strings.Index(pkg, "/")
			stdLib = slash == -1 || strings.Index(pkg[:slash], ".") == -1
		)
		if stdLib {
			continue
		}
		deps[pkg] = struct{}{}
	}

	// Convert from packages to repo roots.
	var depRoots = map[string]*repoRoot{}
	for importPath, _ := range deps {
		var repoRoot, err = repoRootForImportPath(importPath)
		if err != nil {
			perror(err)
		}
		depRoots[repoRoot.root] = repoRoot
	}

	for importPath, repoRoot := range depRoots {
		// TODO: Work with multi-element gopaths
		revision, err := repoRoot.vcs.head(
			path.Join(build.Default.GOPATH, "src", repoRoot.root),
			repoRoot.repo)
		if err != nil {
			perror(err)
		}
		revision = strings.TrimSpace(revision)
		fmt.Println(importPath, revision)
	}
}

// Keep edits to vcs.go separate from the stock version.

var headCmds = map[string]string{
	"git": "rev-parse head",  // 2bebebd91805dbb931317f7a4057e4e8de9d9781
	"hg":  "id",              // 19114a3ee7d5 tip
	"bzr": "log -r-1 --line", // 50: Dimiter Naydenov 2014-02-12 [merge] ec2: Added (Un)AssignPrivateIPAddresses APIs
}

var revisionSeparator = regexp.MustCompile(`[ :]+`)

func (v *vcsCmd) head(dir, repo string) (string, error) {
	var output, err = v.runOutput(dir, headCmds[v.cmd], "dir", dir, "repo", repo)
	if err != nil {
		return "", err
	}
	return revisionSeparator.Split(string(output), -1)[0], nil
}
