GLock is a utility to lock dependencies to specific revisions, integrating with
version control hooks to keep the revision in sync across a team.

* GLOCKFILE - this file lists Go import path and the revision it should be at
* glock - a command line tool for saving and updating dependency versions


Workflow:


Let's say a team works on the shared repository github.com/acme/secret.  To
manage their dependencies, they could follow this workflow:

```
# Record the package's transitive dependencies
# Glock writes the dependencies to a GLOCKFILE in that package's directory.
$ glock deps github.com/acme/secret

# All developers install the git hook, that activates
$ glock install-hook github.com/acme/secret

# Review the dependencies.
# Check it in

# Developer B wants to install
