package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"io/ioutil"
	"os"

	"testing"
)

type saveTest struct {
	name   string
	pkgs   []pkg // pkgs[0] is the target of the save
	output []string
}

type pkg struct {
	importPath string
	files      []file
}

type file struct {
	name    string
	imports []string
}

var saveTests = []saveTest{
	{
		"no third-party deps",
		[]pkg{{
			"github.com/test/p1",
			[]file{
				{"foo.go", []string{"net/http"}},
			},
		},
		},
		[]string{},
	},

	{
		"one third-party dep",
		[]pkg{{
			"github.com/test/p1",
			[]file{
				{"foo.go", []string{"github.com/test/p2"}},
			}}, {
			"github.com/test/p2",
			[]file{
				{"foo.go", []string{"net/http"}},
			}},
		},
		[]string{"github.com/test/p2"},
	},

	{
		"transitive third-party deps",
		[]pkg{{
			"github.com/test/p1",
			[]file{
				{"foo.go", []string{"github.com/test/p2"}},
			}}, {
			"github.com/test/p2",
			[]file{
				{"foo.go", []string{"github.com/test/p3"}},
			}}, {
			"github.com/test/p3",
			[]file{
				{"foo.go", []string{"net/http"}},
			}},
		},
		[]string{"github.com/test/p2", "github.com/test/p3"},
	},

	// the following should be included:
	// - package's tests' dependencies
	//
	// perhaps also higher degree test dependencies
	// - package's dependencies' tests' dependencies
	// - package's tests' dependencies' tests' dependencies
	{
		"test deps",
		[]pkg{{
			"github.com/test/p1",
			[]file{
				{"foo.go", []string{"github.com/test/p2"}},
				{"foo_test.go", []string{"github.com/test/p3"}},
			}}, {
			"github.com/test/p2",
			[]file{
				{"foo.go", []string{"net/http"}},
				{"foo_test.go", []string{"github.com/test/p4"}},
			}}, {
			"github.com/test/p3",
			[]file{
				{"foo.go", []string{"net/http"}},
				{"foo_test.go", []string{"github.com/test/p5"}},
			}}, {
			"github.com/test/p4", // the dependencies' tests' dependency
			[]file{
				{"foo.go", []string{"net/http"}},
			}}, {
			"github.com/test/p5", // the tests' dependencies' tests' dependency
			[]file{
				{"foo.go", []string{"net/http"}},
			}},
		},
		[]string{
			"github.com/test/p2",
			"github.com/test/p3",
			// "github.com/test/p4",  // implement?
			// "github.com/test/p5",
		},
	},

	// FAILS
	// {
	// 	"sub-packages of self",
	// 	[]pkg{{
	// 		"github.com/test/p1",
	// 		[]file{
	// 			{"foo.go", []string{"github.com/test/p1/p2"}},
	// 			{"foo_test.go", []string{"github.com/test/p1/p3"}},
	// 			{"p2/foo.go", []string{"os"}},
	// 			{"p3/foo.go", []string{"os"}},
	// 		}},
	// 	},
	// 	[]string{},
	// },

	{
		"sub-packages",
		[]pkg{{
			"github.com/test/p1",
			[]file{
				{"foo.go", []string{"github.com/test/p2"}},
				{"foo_test.go", []string{"github.com/test/p2/p3"}},
			}}, {
			"github.com/test/p2",
			[]file{
				{"foo.go", []string{"net/http"}},
				{"p3/foo.go", []string{"net/http"}},
			}},
		},
		[]string{
			"github.com/test/p2",
		},
	},
}

func TestSave(t *testing.T) {
	for _, test := range saveTests {
		runSaveTest(t, test)
	}
}

func runSaveTest(t *testing.T, test saveTest) {
	var gopath, err = ioutil.TempDir("", "gopath")
	if err != nil {
		panic(err)
	}
	t.Log(gopath)

	// Create the fake Go packages specified by pkgs
	for _, pkg := range test.pkgs {
		var dir = filepath.Join(gopath, "src", pkg.importPath)
		err = os.MkdirAll(dir, 0777)
		if err != nil {
			panic(err)
		}

		cmd := exec.Command("git", "init")
		cmd.Dir = dir
		gitinit, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("git init: %v\noutput: %v", err, string(gitinit))
			t.FailNow()
		}
		var pkgdir = filepath.Join(gopath, "src", pkg.importPath)
		for _, file := range pkg.files {
			var filedir = filepath.Dir(file.name)
			var filename = filepath.Base(file.name)
			if filedir != "." {
				err = os.MkdirAll(filepath.Join(pkgdir, filedir), 0777)
				if err != nil {
					panic(err)
				}
			}

			var f, err = os.Create(filepath.Join(pkgdir, filedir, filename))
			if err != nil {
				panic(err)
			}
			fmt.Fprintf(f, `package %s

import (
	_ "%s"
)`,
				pkg.importPath[strings.LastIndex(pkg.importPath, "/")+1:],
				strings.Join(file.imports, `"\n	_ "`))
			f.Close()
		}

		cmd = exec.Command("git", "add", ".")
		cmd.Dir = dir
		gitadd, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("git add: %v\noutput: %v", err, string(gitadd))
			t.FailNow()
		}

		cmd = exec.Command("git", "commit", "-am", "initial")
		cmd.Dir = dir
		gitcommit, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("git commit: %v\noutput: %v", err, string(gitcommit))
			t.FailNow()
		}
	}

	// Temporarily set the GOPATH and dep printing function.
	defer os.Setenv("GOPATH", os.Getenv("GOPATH"))
	os.Setenv("GOPATH", gopath)

	var savedPrintDep = printDep
	defer func() {
		printDep = savedPrintDep
	}()

	var buf bytes.Buffer
	printDep = func(importPath, revision string) {
		fmt.Fprintln(&buf, importPath, revision)
	}

	runSave(nil, []string{test.pkgs[0].importPath})

	// See if we got all the expected packages
	var output = buf.String()
	var actual = make(map[string]struct{})
	var scanner = bufio.NewScanner(&buf)
	for scanner.Scan() {
		if txt := scanner.Text(); txt != "" {
			actual[txt[:strings.Index(txt, " ")]] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Error(err)
		return
	}

	if len(actual) != len(test.output) {
		t.Errorf("%v: expected: %v got:\n%v", test.name, test.output, output)
		return
	}

	for _, importPath := range test.output {
		if _, ok := actual[importPath]; !ok {
			t.Errorf("%v: expected: %v got:\n%v", test.name, test.output, output)
			return
		}
	}
}
