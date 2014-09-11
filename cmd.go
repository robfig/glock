package main

import (
	"bufio"
	"fmt"
	"go/build"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

var cmdCmd = &Command{
	UsageLine: "cmd [project import path] [cmd import path]",
	Short:     "add a command to your project's GLOCKFILE",
	Long: `cmd is used to record a Go command-line tool that the project depends on.

The cmd import path must reference a main package. Its dependencies will be
included in the GLOCKFILE along with the project's dependencies, and the command
will be built by "glock sync" and updated by "glock apply".

This functionality allows you to install and update development tools like "vet"
and "errcheck" across a team.  It can even be used to automatically update glock
itself.

Commands are recorded at the top of your GLOCKFILE, in the following format:

	cmd code.google.com/p/go.tools/cmd/godoc
	cmd code.google.com/p/go.tools/cmd/goimports
	cmd code.google.com/p/go.tools/cmd/vet

Options:

	-n	print to stdout instead of writing to file.

`,
}

var cmdN = cmdCmd.Flag.Bool("n", false, "Don't save the file, just print to stdout")

func init() {
	cmdCmd.Run = runCmd // break init loop
}

func runCmd(_ *Command, args []string) {
	if len(args) != 2 {
		cmdCmd.Usage()
		return
	}

	var (
		importPath    = args[0]
		cmd           = args[1]
		glockFilename = path.Join(os.Getenv("GOPATH"), "src", importPath, "GLOCKFILE")
	)

	// Import the cmd to ensure it exists and is a main package.
	pkg, err := build.Import(cmd, "", 0)
	if err != nil {
		perror(fmt.Errorf("Failed to import %v: %v", cmd, err))
	}
	if pkg.Name != "main" {
		perror(fmt.Errorf("Found package %v, expected main", pkg.Name))
	}

	// Build it
	installOutput, err := run("go", "install", "-v", cmd)
	if err != nil {
		perror(fmt.Errorf("Failed to build %v:\n%v", cmd, string(installOutput)))
	}

	// Read the existing GLOCKFILE into cmds, liblines
	var cmds, liblines []string
	glockfile, err := os.Open(glockFilename)
	if err != nil {
		perror(err)
	}
	var scanner = bufio.NewScanner(glockfile)
	for scanner.Scan() {
		var txt = scanner.Text()
		var fields = strings.Fields(txt)
		if len(fields) != 2 {
			continue
		}
		if fields[0] == "cmd" {
			cmds = append(cmds, fields[1])
		} else {
			liblines = append(liblines, txt)
		}
	}
	if err := scanner.Err(); err != nil {
		perror(err)
	}
	glockfile.Close()

	// Get an output writer for the new GLOCKFILE contents
	var output io.Writer = os.Stdout
	if !*cmdN {
		var f, err = os.Create(glockFilename)
		if err != nil {
			perror(fmt.Errorf("error creating %s: %v", glockFilename, err))
		}
		defer f.Close()
		output = f
	}

	// Write cmds and libraries out.
	cmds = append(cmds, cmd)
	outputCmds(output, cmds)

	sort.Strings(liblines)
	for _, libline := range liblines {
		fmt.Fprintln(output, libline)
	}
}
