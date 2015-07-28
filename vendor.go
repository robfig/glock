package main

import (
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path"
)

type vendorInfo struct {
	vendored   bool
	origGopath string
	pathToRm   string
}

// maybeVendorGOPATH arranges so that "go get" fetches into project/vendor:
// - Create project/vendor directory if necessary
// - Create a temporary directory symlinked to it, e.g. /tmp/glock-123/src
// - Symlink the project directory itself into it, e.g. project/vendor/project
// - Set GOPATH to TMPDIR.
//
// If necessary, a path to remove at the conclusion of the operation is returned.
// Provide it as an argument to cleanupVendorGOPATH
func maybeVendorGOPATH(importPath string) vendorInfo {
	if os.Getenv("GO15VENDOREXPERIMENT") == "" {
		return vendorInfo{vendored: false}
	}
	origGopath := gopath()
	projectPath := path.Join(origGopath, "src", importPath)
	vendorPath := path.Join(projectPath, "vendor")
	tmpGopath, err := ioutil.TempDir(os.TempDir(), "glock")
	check(err)
	check(os.MkdirAll(vendorPath, 0755))
	check(os.MkdirAll(tmpGopath, 0755))
	check(os.Symlink(vendorPath, path.Join(tmpGopath, "src")))
	check(os.MkdirAll(path.Join(vendorPath, path.Dir(importPath)), 0755))
	check(os.Symlink(projectPath, path.Join(vendorPath, importPath)))
	check(os.Setenv("GOPATH", tmpGopath))
	build.Default.GOPATH = tmpGopath
	fmt.Println("TMP GOPATH: ", tmpGopath)
	return vendorInfo{true, origGopath, path.Join(vendorPath, importPath)}
}

// cleanupVendorGOPATH accepts the output of maybeVendorGOPATH and cleans up
// afterwards.
func cleanupVendorGOPATH(info vendorInfo) {
	if info.vendored {
		build.Default.GOPATH = info.origGopath
		check(os.Setenv("GOPATH", info.origGopath))
		check(os.Remove(info.pathToRm))
	}
}
