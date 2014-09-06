package main

import (
	"fmt"
	"go/build"
	"io/ioutil"
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

const gitHook = `#!/bin/bash
set -e

if [[ $GIT_REFLOG_ACTION != pull* ]]; then
        exit 0
fi

LOG=$(git log -U0 --oneline -p HEAD@{1}..HEAD GLOCKFILE)
[ -z "$LOG" ] && echo "glock: no changes to apply" && exit 0
echo "glock: applying updates..."
glock apply <<< "$LOG"
`

type hook struct{ filename, content string }

var vcsHooks = map[*vcsCmd][]hook{
	vcsGit: {
		{filepath.Join(".git", "hooks", "post-merge"), gitHook},    // git pull
		{filepath.Join(".git", "hooks", "post-checkout"), gitHook}, // git pull --rebase
	},
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
	var hooks, ok = vcsHooks[repo.vcs]
	if !ok {
		perror(fmt.Errorf("%s hook not implemented", repo.vcs.name))
	}

	pkg, err := build.Import(repo.root, "", build.FindOnly)
	if err != nil {
		perror(fmt.Errorf("Failed to import %v: %v", repo.root, err))
	}

	for _, hook := range hooks {
		var filename = filepath.Join(pkg.Dir, hook.filename)
		var err = os.MkdirAll(filepath.Dir(filename), 0755)
		if err != nil {
			perror(err)
		}
		err = ioutil.WriteFile(filename, []byte(hook.content), 0755)
		if err != nil {
			perror(err)
		}
		fmt.Println("Installed", filename)
	}
}
