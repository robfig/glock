GLock is a utility to lock dependencies to specific revisions, integrating with
version control hooks to keep those revisions in sync across a team, without
vendoring them (rewriting imports and checking them in with your source code).

* GLOCKFILE - this file lists a Go import path and the revision it should be at
* glock - a command line tool for saving and updating dependency versions

## Setup

Here is how to get started with Glock.

```
# Fetch and install glock
$ go get github.com/robfig/glock

# Record the package's transitive dependencies, as they currently exist.
# Glock writes the dependencies to a GLOCKFILE in that package's directory.
$ glock save github.com/acme/project

# Review and check in the dependencies.
$ git add src/github.com/acme/project/GLOCKFILE
$ git commit -m 'Save current dependency revisions'
$ git push

# All developers install the git hook
$ glock install-hook github.com/acme/project
```

## Ongoing workflow

After initial setup, here is how the workflow goes:

```
# Developer wants to add a dependency
# (Edit code to use dependency)
$ go get github.com/some/dependency
$ glock save github.com/acme/project
$ git commit src/github.com/acme/project/GLOCKFILE && git push

# Developer wants to update all dependencies
```

Other developers will have their dependencies added and updated automatically,
when the git hook notices a change to the GLOCKFILE.
