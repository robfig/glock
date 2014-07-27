GLock is a command-line tool to lock dependencies to specific revisions, using a
version control hook to keep those revisions in sync across a team.

The process works as follows:

* "glock save" a list of current dependencies and their revisions to a GLOCKFILE
* "glock install" the git hook
* As other developers update dependencies, the hook applies those changes
  automatically.

This approach is easy on developers and avoids repository bloat from third-party
dependencies.

It is meant to serve a team that:

* develops multiple applications within a single Go codebase
* uses a single dedicated GOPATH for development
* wants all applications within the codebase to use one version of any dependency.


## Setup

Here is how to get started with Glock.

```
# Fetch and install glock
$ go get github.com/robfig/glock

# Record the package's transitive dependencies, as they currently exist.
# Glock writes the dependencies to a GLOCKFILE in that package's directory.
# All depenencies of all descendent packages are included.
$ glock save github.com/acme/project

# Review and check in the dependencies.
$ git add src/github.com/acme/project/GLOCKFILE
$ git commit -m 'Save current dependency revisions'
$ git push

# All developers install the git hook
$ glock install github.com/acme/project
```

Once the VCS hook is installed, all developers will have their dependencies
added and updated automatically as the GLOCKFILE changes.

## Add/update a dependency

```
# Developer wants to add a dependency
$ go get github.com/some/dependency
$ glock save github.com/acme/project
$ git commit src/github.com/acme/project/GLOCKFILE
$ git push
```

The dependency update will be propagated to all team members as they pull that
revision.
