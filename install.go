package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var cmdInstall = &Command{
	UsageLine: "install [import path]",
	Short:     "add a post-merge hook that applies GLOCKFILE changes after each pull.",
	Long: `Install adds a glock hook to the given package's repository

When pulling new commits, it checks whether the GLOCKFILE has been updated. If so,
it calls "glock apply", passing in the diff.`,
}

func init() {
	cmdInstall.Run = runInstall // break init loop
}

const gitPostMergeHook = `#!/bin/bash
set -e

LOG=$(git log -U0 --oneline -p ORIG_HEAD..HEAD GLOCKFILE)
[ -z "$LOG" ] && echo "glock: no changes to apply" && exit 0
# TODO: Test if glock is on path.  If not, go get it.
glock apply <<< "$LOG"
`

type hook struct{ filename, content string }

var vcsHooks = map[*vcsCmd]hook{
	vcsGit: {filepath.Join(".git", "hooks", "post-merge"), gitPostMergeHook},
}

func runInstall(cmd *Command, args []string) {
	if len(args) == 0 {
		cmdInstall.Usage()
		return
	}
	var importPath = args[0]
	var repo, err = glockRepoRootForImportPath(importPath)
	if err != nil {
		perror(err)
	}
	var hook, ok = vcsHooks[repo.vcs]
	if !ok {
		perror(fmt.Errorf("%s hook not implemented", repo.vcs.name))
	}
	var filename = filepath.Join(repo.root, hook.filename)
	f, err := os.Create(filename)
	if err != nil {
		perror(err)
	}
	f.Write([]byte(hook.content))
	f.Close()
	fmt.Println("Installed", filename)
}
