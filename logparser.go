package main

import (
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

type libraryAction struct {
	action               action
	importPath, revision string
}

type cmdAction struct {
	add        bool
	importPath string
}

type playbook struct {
	library []libraryAction // updates to libraries in gopath
	cmd     []cmdAction     // updates to cmds that should be built
}

const (
	importPathExpr = `[\w.]+\.\w+/[\w/.-]+`
	libLineExpr    = `[+-](` + importPathExpr + `) (\w+)`
	cmdLineExpr    = `[+-]cmd (` + importPathExpr + `)`
)

var (
	libLineRegex = regexp.MustCompile(libLineExpr)
	cmdLineRegex = regexp.MustCompile(cmdLineExpr)
)

func readDiffLines(reader io.Reader) []diff {
	// Get the list of diffs from the commit log.
	var (
		diffs   []diff
		scanner = bufio.NewScanner(reader)
	)
	for scanner.Scan() {
		var txt = scanner.Text()
		if matches := libLineRegex.FindStringSubmatch(txt); matches != nil {
			diffs = append(diffs, diff{
				importPath: matches[1],
				revision:   matches[2],
				added:      txt[0] == '+',
			})
		} else if matches := cmdLineRegex.FindStringSubmatch(txt); matches != nil {
			diffs = append(diffs, diff{
				importPath: matches[1],
				revision:   "cmd",
				added:      txt[0] == '+',
			})
		} else {
			diffs = append(diffs, emptyLine)
		}
	}
	return diffs
}

func buildPlaybook(diffs []diff) playbook {
	// Convert diffs into actions.  Since they may touch the same lines over
	// multiple commits, keep track of import paths that we've added commands for,
	// and only add the first.
	var (
		book            playbook
		seenImportPaths = make(map[string]struct{})
	)
	for i := 0; i < len(diffs); i++ {
		var this = diffs[i]
		if this == emptyLine {
			continue
		}
		if this.revision == "cmd" {
			book.cmd = append(book.cmd, cmdAction{this.added, this.importPath})
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
			book.library = append(book.library, newUpdate(this, next))
			i++
			continue
		}

		// If not, record this as an action.
		book.library = append(book.library, newAddOrRemove(this))
	}
	return book
}

func newUpdate(a, b diff) libraryAction {
	var added = a
	if b.added {
		added = b
	}
	return newCommand(update, added)
}

func newAddOrRemove(d diff) libraryAction {
	var action = add
	if !d.added {
		action = remove
	}
	return newCommand(action, d)
}

func newCommand(a action, d diff) libraryAction {
	return libraryAction{
		action:     a,
		importPath: d.importPath,
		revision:   d.revision,
	}
}
