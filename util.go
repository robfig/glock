package main

import (
	"errors"
	"fmt"
	"go/build"
	"io"
	"os"
	"path"
	"path/filepath"
)

// glockRepoRootForImportPath wraps the vcs.go version.  If the stock one
// doesn't work, it looks for .git, .hg directories up to the tree.
// This is done to support repos with non-go-get friendly names.
// Also, returns an error if it doesn't exist (e.g. it needs to be go gotten).
func glockRepoRootForImportPath(importPath string) (*repoRoot, error) {
	var pkg, err = build.Import(importPath, "", build.FindOnly)
	if err != nil {
		return nil, err
	}

	rr, err := repoRootForImportPath(importPath)
	if err == nil {
		// it may not exist, even with err == nil
		_, err = os.Stat(pkg.Dir)
		if err != nil {
			return nil, err
		}
		return rr, nil
	}

	var dir = pkg.ImportPath
	for len(dir) > 1 {
		rr, err := fastRepoRoot(dir)
		if err == nil {
			return rr, nil
		}
		dir = filepath.Dir(dir)
	}

	return nil, fmt.Errorf("no version control directory found for %q", importPath)
}

// fastRepoRoot just checks for existence of VCS dot directories to determine
// which VCS to use for the given import path.
// If none are found, an error is returned.
func fastRepoRoot(rootImportPath string) (*repoRoot, error) {
	var pkg, err = build.Import(rootImportPath, "", build.FindOnly)
	if err != nil {
		return nil, err
	}

	for _, vcsDir := range []string{".git", ".hg", ".bzr", ".svn"} {
		_, err := os.Stat(filepath.Join(pkg.Dir, vcsDir))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		vcs := vcsByCmd(vcsDir[1:])
		if vcs == nil {
			return nil, fmt.Errorf("unknown version control system %q", vcsDir[1:])
		}
		return &repoRoot{
			vcs:  vcs,
			repo: "",
			root: rootImportPath,
		}, nil
	}
	return nil, errors.New("no repo found")
}

func gopath() string {
	return filepath.SplitList(build.Default.GOPATH)[0]
}

func glockFilename(importPath string) string {
	return path.Join(gopath(), "src", importPath, "GLOCKFILE")
}

func glockfileReader(importPath string, n bool) io.ReadCloser {
	if n {
		return os.Stdin
	}

	var glockfile, err = os.Open(glockFilename(importPath))
	if err != nil {
		perror(err)
	}
	return glockfile
}

func glockfileWriter(importPath string, n bool) io.WriteCloser {
	if n {
		return os.Stdout
	}

	var f, err = os.Create(glockFilename(importPath))
	if err != nil {
		perror(fmt.Errorf("error creating %s: %v", glockFilename, err))
	}
	return f
}
