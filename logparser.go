package main

import (
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path"
	"strings"

	"bufio"

	"io"

	"regexp"
)

// diff represents a line of difference in a commit.
// the zero value represent non-matching lines in the diff.
// (this is necessary to distinguish an update from adding in one commit and
// removing in the next)
type diff struct {
	importPath, revision string
	added                bool
}

var emptyLine = diff{}

type action int

const (
	add action = iota
	remove
	update
)

type command struct {
	action               action
	importPath, revision string
}

func readDiffLines(reader io.Reader) []diff {
	// Get the list of diffs from the commit log.
	var (
		diffs     []diff
		scanner   = bufio.NewScanner(reader)
		lineRegex = regexp.MustCompile(`[+-]([\w.]+\.\w+/[\w/-]+) (\w+)`)
	)
	for scanner.Scan() {
		if !lineRegex.MatchString(scanner.Text()) {
			diffs = append(diffs, emptyLine)
			continue
		}

		var matches = lineRegex.FindStringSubmatch(scanner.Text())
		diffs = append(diffs, diff{
			importPath: matches[1],
			revision:   matches[2],
			added:      scanner.Text()[0] == '+',
		})
	}
	return diffs
}

func buildCommands(diffs []diff) []command {
	// Convert diffs into actions.  Since they may touch the same lines over
	// multiple commits, keep track of import paths that we've added commands for,
	// and only add the first.
	var (
		cmds            []command
		seenImportPaths = make(map[string]struct{})
	)
	for i := 0; i < len(diffs); i++ {
		var this = diffs[i]
		if this == emptyLine {
			continue
		}
		if _, ok := seenImportPaths[this.importPath]; ok {
			continue
		}
		seenImportPaths[this.importPath] = struct{}{}

		// Is this an updated line pair (which we treat as a unit)?
		var next = emptyLine
		if i < len(diffs)-1 {
			next = diffs[i+1]
		}
		if next != emptyLine && next.importPath == this.importPath {
			if this.added == next.added {
				panic("most unexpected")
			}
			cmds = append(cmds, newUpdate(this, next))
			i++
			continue
		}

		// If not, record this as an action.
		cmds = append(cmds, newAddOrRemove(this))
	}
	return cmds
}

func main() {
	var gopath = build.Default.SplitPathList(build.Default.GOPATH)[0]
	var cmds = buildCommands(readDiffLines(os.Stdin))
	for _, cmd := range cmds {
		var importDir = path.Join(gopath, "src", cmd.importPath)
		switch cmd.action {
		case remove:
			fmt.Println(cmd.importPath, "is no longer in use.  Delete? [Y/n]")
			var answer string
			fmt.Scanln(&answer)
			if answer == "" || answer == "y" || answer == "Y" {
				os.RemoveAll(importDir)
			}
		case add:
			exec.Command("go", "get", cmd.importPath).Wait()
			fallthrough
		case update:
			os.Chdir(importDir)
			var firstSlash = strings.Index(cmd.importPath, "/")
			if firstSlash == -1 {
				fmt.Println("Failed to parse import path")
				continue
			}
			var output []byte
			var err error
			var vcs []string
			switch cmd.importPath[:firstSlash] {
			case "github.com":
				vcs = []string{"git", "reset", "--hard", cmd.revision}
			case "code.google.com", "bitbucket.org":
				vcs = []string{"hg", "update", "-f", cmd.revision}
			case "launchpad.net":
				vcs = []string{"bzr", "update", "-r", cmd.revision}
			default:
				fmt.Println("Not sure which VCS to use for:", cmd.importPath)
				continue
			}

			output, err = exec.Command(vcs...).CombinedOutput()
			fmt.Println(string(output))
			if err != nil {
				fmt.Println("Error:", err)
			}
		}
	}
}

func newUpdate(a, b diff) command {
	var added = a
	if b.added {
		added = b
	}
	return newCommand(update, added)
}

func newAddOrRemove(d diff) command {
	var action = add
	if !d.added {
		action = remove
	}
	return newCommand(action, d)
}

func newCommand(a action, d diff) command {
	return command{
		action:     a,
		importPath: d.importPath,
		revision:   d.revision,
	}
}
