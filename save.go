package main

import (
	"bufio"
	"errors"
	"fmt"
	"go/build"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/buildutil"
)

var cmdSave = &Command{
	UsageLine: "save [import path]",
	Short:     "save a GLOCKFILE for the given package's dependencies",
	Long: `save is used to record the current revisions of a package's dependencies

It writes this state to a file in the root of the package called "GLOCKFILE".

Options:

	-n	print to stdout instead of writing to file.

`,
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

	// Read cmd lines from GLOCKFILE and calculate required dependencies.
	var (
		importPath = args[0]
		cmds       = readCmds(importPath)
		depRoots   = calcDepRoots(importPath, cmds)
	)

	output := glockfileWriter(importPath, *saveN)
	outputCmds(output, cmds)
	outputDeps(output, depRoots)
	output.Close()
}

func outputCmds(w io.Writer, cmds []string) {
	sort.Strings(cmds)
	var prev string
	for _, cmd := range cmds {
		if cmd != prev {
			fmt.Fprintln(w, "cmd", cmd)
		}
		prev = cmd
	}
}

func outputDeps(w io.Writer, depRoots []*repoRoot) {
	for _, repoRoot := range depRoots {
		revision, err := repoRoot.vcs.head(repoRoot.path, repoRoot.repo)
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
func calcDepRoots(importPath string, cmds []string) []*repoRoot {
	var attempts = 1
GetAllDeps:
	var depRoots = map[string]*repoRoot{}
	var missingPackages []string
	for _, importPath := range getAllDeps(importPath, cmds) {
		// Convert from packages to repo roots.
		// TODO: Filter out any packages that have prefixes also included in the list.
		// e.g. pkg/foo/bar , pkg/foo/baz , pkg/foo
		// That would skip the relatively slow (redundant) determining of repo root for each.
		var repoRoot, err = glockRepoRootForImportPath(importPath)
		if err != nil {
			perror(err)
		}

		// Ensure we have the package locally. if not, we don't have all the possible deps.
		_, err = build.Import(repoRoot.root, "", build.FindOnly)
		if err != nil {
			missingPackages = append(missingPackages, repoRoot.root)
		}

		depRoots[repoRoot.root] = repoRoot
	}

	// If there were missing packages, try again.
	if len(missingPackages) > 0 {
		if attempts > 3 {
			perror(fmt.Errorf("failed to fetch missing packages: %v", missingPackages))
		}
		fmt.Fprintln(os.Stderr, "go", "get", "-d", strings.Join(missingPackages, " "))
		run("go", append([]string{"get", "-d"}, missingPackages...)...)
		attempts++
		goto GetAllDeps
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

var majorVersionComponent = regexp.MustCompile(`v[\d]+`)

// pathWithoutMajorVersion returns path with the 1st major version /component
// (if any) stripped out. If one was found, the 2nd return value is true.
func pathWithoutMajorVersion(path string) (string, bool) {
	parts := strings.Split(path, "/")
	for idx, part := range strings.Split(path, "/") {
		if majorVersionComponent.MatchString(part) {
			return strings.Join(append(parts[:idx], parts[idx+1:]...), "/"), true
		}
	}
	return path, false
}

// tryImport attempts to import the path as-is and, if it fails to be found and
// path contains a major module version, reattempts with the version removed.
func tryImport(ctx build.Context, path, srcDir string, mode build.ImportMode) (*build.Package, error) {
	pkg, err := ctx.Import(path, srcDir, mode)
	if err != nil && strings.HasPrefix(err.Error(), "cannot find package ") {
		if versionlessPath, ok := pathWithoutMajorVersion(path); ok {
			return ctx.Import(versionlessPath, srcDir, mode)
		}
	}
	return pkg, err
}

// getAllDeps returns a slice of package import paths for all dependencies
// (including test dependencies) of the given import path (and subpackages) and commands.
func getAllDeps(importPath string, cmds []string) []string {
	subpackagePrefix := importPath + "/"

	var depsSlice []string
	for _, useAllFiles := range []bool{false, true} {
		printLoadingError := func(path string, err error) {
			if err != nil && !useAllFiles {
				// Lots of errors because of UseAllFiles.
				log.Printf("error loading package %s: %s", path, err)
			}
		}

		deps := map[string]struct{}{}
		roots := map[string]struct{}{
			importPath: {},
		}

		// Add the command packages. Note that external command packages are
		// considered dependencies.
		for _, pkg := range cmds {
			roots[pkg] = struct{}{}

			if !strings.HasPrefix(pkg, subpackagePrefix) {
				deps[pkg] = struct{}{}
			}
		}

		buildContext := build.Default
		buildContext.CgoEnabled = true
		buildContext.UseAllFiles = useAllFiles

		// Add the subpackages.
		for path := range buildutil.ExpandPatterns(&buildContext, []string{subpackagePrefix + "..."}) {
			_, err := tryImport(buildContext, path, "", 0)
			if _, ok := err.(*build.NoGoError); ok {
				continue
			}
			printLoadingError(path, err)
			roots[path] = struct{}{}
		}

		var addTransitiveClosure func(string)
		addTransitiveClosure = func(path string) {
			pkg, err := tryImport(buildContext, path, "", 0)
			printLoadingError(path, err)

			importPaths := append([]string(nil), pkg.Imports...)
			if _, ok := roots[path]; ok {
				importPaths = append(importPaths, pkg.TestImports...)
				importPaths = append(importPaths, pkg.XTestImports...)
			}

			for _, path := range importPaths {
				if path == "C" {
					continue // "C" is fake
				}

				// Resolve the import path relative to the importing package.
				if bp2, _ := tryImport(buildContext, path, pkg.Dir, build.FindOnly); bp2 != nil {
					path = bp2.ImportPath
				}

				// Exclude our roots. Note that commands are special-cased above.
				if _, ok := roots[path]; ok {
					continue
				}
				slash := strings.IndexByte(path, '/')
				stdLib := slash == -1 || strings.IndexByte(path[:slash], '.') == -1
				// Exclude the standard library.
				if stdLib {
					continue
				}
				if _, ok := deps[path]; !ok {
					deps[path] = struct{}{}
					addTransitiveClosure(path)
				}
			}
		}

		for path := range roots {
			addTransitiveClosure(path)
		}
		addTransitiveClosure(importPath)

		depsSlice = append(depsSlice, setToSlice(deps)...)
	}

	return depsSlice
}

func run(name string, args ...string) ([]byte, error) {
	if buildV {
		fmt.Println(name, args)
	}
	var cmd = exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func setToSlice(set map[string]struct{}) []string {
	var slice []string
	for k := range set {
		slice = append(slice, k)
	}
	return slice
}

// readCmds returns the list of cmds declared in the given glockfile.
// They must appear at the top of the file, with the syntax:
//   cmd code.google.com/p/go.tools/cmd/godoc
//   cmd github.com/robfig/glock
func readCmds(importPath string) []string {
	var (
		glockfile io.ReadCloser
		err       error
	)
	for _, gopath := range gopaths() {
		glockfile, err = os.Open(glockFilename(gopath, importPath))
		if err == nil {
			break
		}
	}
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		perror(err)
	}

	var cmds []string
	var scanner = bufio.NewScanner(glockfile)
	for scanner.Scan() {
		var fields = strings.Fields(scanner.Text())
		if len(fields) != 2 || fields[0] != "cmd" {
			return cmds
		}
		cmds = append(cmds, fields[1])
	}
	if err := scanner.Err(); err != nil {
		perror(err)
	}
	return cmds
}

// Keep edits to vcs.go separate from the stock version.

var headCmds = map[string]string{
	"git": "rev-parse HEAD",  // 2bebebd91805dbb931317f7a4057e4e8de9d9781
	"hg":  "id",              // 19114a3ee7d5+ tip
	"bzr": "log -r-1 --line", // 50: Dimiter Naydenov 2014-02-12 [merge] ec2: Added (Un)AssignPrivateIPAddresses APIs
}

var (
	revisionSeparator = regexp.MustCompile(`[ :+]+`)
	validRevision     = regexp.MustCompile(`^[\d\w]+$`)
)

func (v *vcsCmd) head(dir, repo string) (string, error) {
	var output, err = v.runOutput(dir, headCmds[v.cmd], "dir", dir, "repo", repo)
	if err != nil {
		return "", err
	}
	return parseHEAD(output)
}

func parseHEAD(output []byte) (string, error) {
	// Handle a case where HG returns success but prints an error, causing our
	// parsing of the revision id to break.
	var str = strings.TrimSpace(string(output))
	for strings.HasPrefix(str, "*** ") {
		var i = strings.Index(str, "\n")
		if i == -1 {
			break
		}
		str = str[i+1:]
	}

	var head = revisionSeparator.Split(str, -1)[0]
	if !validRevision.MatchString(head) {
		fmt.Fprintln(os.Stderr, string(output))
		return "", errors.New("error getting head revision")
	}
	return head, nil
}
