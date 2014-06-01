package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
)

var cmdSave = &Command{
	UsageLine: "save [package]",
	Short:     "save a GLOCKFILE for the given package's dependencies",
	Long:      `TODO`,
}

var saveN = cmdSave.Flag.Bool("n", false, "Don't save the file, just print to stdout")

func init() {
	cmdSave.Run = runSave // break init loop
}

func runSave(cmd *Command, args []string) {
	if len(args) == 0 {
		cmdSave.Usage()
		return
	}
	var importPath = args[0]
	var depRoots = calcDepRoots(importPath)

	// Figure out where output should go
	var output io.Writer = os.Stdout
	if !*saveN {
		var glockFilename = path.Join(os.Getenv("GOPATH"), "src", importPath, "GLOCKFILE")
		var f, err = os.Create(glockFilename)
		if err != nil {
			perror(fmt.Errorf("error creating %s: %v", glockFilename, err))
		}
		defer f.Close()
		output = f
	}

	outputDeps(output, depRoots)
}

func outputDeps(w io.Writer, depRoots []*repoRoot) {
	for _, repoRoot := range depRoots {
		// TODO: Work with multi-element gopaths
		revision, err := repoRoot.vcs.head(
			path.Join(os.Getenv("GOPATH"), "src", repoRoot.root),
			repoRoot.repo)
		if err != nil {
			perror(err)
		}
		revision = strings.TrimSpace(revision)
		fmt.Fprintln(w, repoRoot.root, revision)
	}
}

// calcDepRoots discovers all dependencies of the given importPath and returns
// them as a list of the repo roots that cover all dependent packages. for
// example, github.com/robfig/soy and github.com/robfig/soy/data are two
// dependent packages but only one repo. the returned repos are ordered
// alphabetically by import path.
func calcDepRoots(importPath string) []*repoRoot {
	// Validate that we got an import path that is the base of a repo.
	var repo, err = repoRootForImportPath(importPath)
	if err != nil {
		perror(err)
	}
	if repo.root != importPath {
		perror(fmt.Errorf("%v must be the base of a repo", importPath))
	}

	// Convert from packages to repo roots.
	var depRoots = map[string]*repoRoot{}
	for _, importPath := range getAllDeps(importPath) {
		var repoRoot, err = repoRootForImportPath(importPath)
		if err != nil {
			perror(err)
		}
		depRoots[repoRoot.root] = repoRoot
	}

	// Remove any dependencies to packages within the target repo
	delete(depRoots, importPath)

	var repos []*repoRoot
	for _, repo := range depRoots {
		repos = append(repos, repo)
	}
	sort.Sort(byImportPath(repos))
	return repos
}

type byImportPath []*repoRoot

func (p byImportPath) Len() int           { return len(p) }
func (p byImportPath) Less(i, j int) bool { return p[i].root < p[j].root }
func (p byImportPath) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// getAllDeps returns a slice of package import paths for all dependencies
// (including test dependencies) of the given package.
func getAllDeps(importPath string) []string {
	// Get a set of transitive dependencies (package import paths) for the
	// specified package.
	var output = run("go", "list", "-f", `{{range .Deps}}{{.}}{{"\n"}}{{end}}`, importPath)
	var deps = filterPackages(output, nil) // filter out standard library

	// List dependencies of test files, which are not included in the go list .Deps
	// Also, ignore any dependencies that are already covered.
	var testImportOutput = run("go", "list", "-f", `{{range .TestImports}}{{.}}{{"\n"}}{{end}}`, importPath)
	var testImmediateDeps = filterPackages(testImportOutput, deps) // filter out standard library and existing deps
	for dep := range testImmediateDeps {
		deps[dep] = struct{}{}
	}

	// We have to get the transitive deps of the remaining test imports.
	// NOTE: this will return the dependencies of the libraries imported by tests
	// and not imported by main code.  This output does not include the imports themselves.
	var testDepOutput = run("go", append([]string{"list", "-f", `{{range .Deps}}{{.}}{{"\n"}}{{end}}`}, setToSlice(testImmediateDeps)...)...)
	var allTestDeps = filterPackages(testDepOutput, deps) // filter out standard library and existing deps
	for dep := range allTestDeps {
		deps[dep] = struct{}{}
	}

	// Return everything in deps
	var result []string
	for dep := range deps {
		result = append(result, dep)
	}
	return result
}

// run is a wrapper for exec.Command(..).CombinedOutput() that provides helpful
// error message and exits on failure.
func run(name string, args ...string) []byte {
	if buildV {
		fmt.Println(name, args)
	}
	var cmd = exec.Command(name, args...)
	cmd.Env = []string{"GOPATH=" + os.Getenv("GOPATH")}
	var output, err = cmd.CombinedOutput()
	if err != nil {
		perror(fmt.Errorf("%v %v\n%v\nError: %v", name, args, string(output), err))
	}
	return output
}

func setToSlice(set map[string]struct{}) []string {
	var slice []string
	for k := range set {
		slice = append(slice, k)
	}
	return slice
}

// filterPackages accepts the output of a go list comment (one package per line)
// and returns a set of package import paths, excluding standard library.
// Additionally, any packages present in the "exclude" set will be excluded.
func filterPackages(output []byte, exclude map[string]struct{}) map[string]struct{} {
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
		if _, ok := exclude[pkg]; ok {
			continue
		}
		deps[pkg] = struct{}{}
	}
	return deps
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
