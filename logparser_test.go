package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const templateWithContext = `25b4da1 change lgo4go to bogus revision
diff --git a/REVISIONS b/REVISIONS
index 82ef4f5..efc84fa 100644
--- a/REVISIONS
+++ b/REVISIONS
@@ -1,6 +1,6 @@
 code.google.com/p/go.net bc411e2ac33f
 code.google.com/p/goprotobuf 4794f7baff22
%s
 github.com/cactus/go-statsd-client c244f509a1c4e71828484fc2d09b8cfd7407795d
 github.com/codegangsta/inject 346a984957aa24276ebc5e7b16b3ac6a50fe8138
 github.com/codegangsta/martini/ 50f2d3f9d7eebef98b1a5dcf29dad72c66e918a7
`

const templateNoContext = `25b4da1 change lgo4go to bogus revision
diff --git a/REVISIONS b/REVISIONS
index 82ef4f5..efc84fa 100644
--- a/REVISIONS
+++ b/REVISIONS
@@ -1,6 +1,6 @@
%s
`

var tests = []struct {
	name  string
	diffs []string // one per "commit"
	book  playbook
}{
	{"update existing", []string{`
-code.google.com/p/log4go c3294304d93f
+code.google.com/p/log4go 4794f7baff22
`}, playbook{library: []libraryAction{
		{update, "code.google.com/p/log4go", "4794f7baff22"},
	}}},

	{"update existing (add first)", []string{`
+code.google.com/p/log4go 4794f7baff22
-code.google.com/p/log4go c3294304d93f
`}, playbook{library: []libraryAction{
		{update, "code.google.com/p/log4go", "4794f7baff22"},
	}}},

	{"update existing block", []string{`
-code.google.com/p/log4go c3294304d93f
-code.google.com/p/log4go2 4794f7baff22
+code.google.com/p/log4go 4794f7baff22
+code.google.com/p/log4go2 c3294304d93f
`}, playbook{library: []libraryAction{
		{update, "code.google.com/p/log4go", "4794f7baff22"},
		{update, "code.google.com/p/log4go2", "c3294304d93f"},
	}}},

	{"update existing block (add first)", []string{`
+code.google.com/p/log4go 4794f7baff22
+code.google.com/p/log4go2 c3294304d93f
-code.google.com/p/log4go c3294304d93f
-code.google.com/p/log4go2 4794f7baff22
`}, playbook{library: []libraryAction{
		{update, "code.google.com/p/log4go", "4794f7baff22"},
		{update, "code.google.com/p/log4go2", "c3294304d93f"},
	}}},

	{"update existing block (intermixed)", []string{`
+code.google.com/p/log4go 4794f7baff22
-code.google.com/p/log4go c3294304d93f
+code.google.com/p/log4go2 c3294304d93f
-code.google.com/p/log4go2 4794f7baff22
`}, playbook{library: []libraryAction{
		{update, "code.google.com/p/log4go", "4794f7baff22"},
		{update, "code.google.com/p/log4go2", "c3294304d93f"},
	}}},

	{"update existing block (intermixed 2)", []string{`
+code.google.com/p/log4go 4794f7baff22
-code.google.com/p/log4go2 4794f7baff22
+code.google.com/p/log4go2 c3294304d93f
-code.google.com/p/log4go c3294304d93f
`}, playbook{library: []libraryAction{
		{update, "code.google.com/p/log4go", "4794f7baff22"},
		{update, "code.google.com/p/log4go2", "c3294304d93f"},
	}}},

	{"add dep", []string{`
+code.google.com/p/log4go 4794f7baff22
`}, playbook{library: []libraryAction{
		{add, "code.google.com/p/log4go", "4794f7baff22"},
	}}},

	{"remove dep", []string{`
-code.google.com/p/log4go 4794f7baff22
`}, playbook{library: []libraryAction{
		{remove, "code.google.com/p/log4go", "4794f7baff22"},
	}}},

	{"no change", []string{""}, playbook{}},

	{"multiple commits", []string{`
+code.google.com/p/log4go 4794f7baff22
`, `
+launchpad.net/gocheck 45
`}, playbook{library: []libraryAction{
		{add, "code.google.com/p/log4go", "4794f7baff22"},
		{add, "launchpad.net/gocheck", "45"},
	}}},

	{"multiple commits on same import path", []string{`
-code.google.com/p/log4go 2
+code.google.com/p/log4go 3
`, `
-code.google.com/p/log4go 1
+code.google.com/p/log4go 2
`}, playbook{library: []libraryAction{
		{update, "code.google.com/p/log4go", "3"},
	}}},

	{"exotic repo names", []string{`
+code.google.com/p/go.tools 1
+github.com/shurcooL/Go_Package-Store 1
`}, playbook{library: []libraryAction{
		{add, "code.google.com/p/go.tools", "1"},
		{add, "github.com/shurcooL/Go_Package-Store", "1"},
	}}},

	{"add cmd", []string{`
+cmd code.google.com/p/go.tools/cmd/godoc
`}, playbook{cmd: []cmdAction{
		{true, "code.google.com/p/go.tools/cmd/godoc"},
	}}},

	{"remove cmd", []string{`
-cmd code.google.com/p/go.tools/cmd/godoc
`}, playbook{cmd: []cmdAction{
		{false, "code.google.com/p/go.tools/cmd/godoc"},
	}}},
}

func TestLogParser(t *testing.T) {
	for _, test := range tests {
		for _, tmpl := range []string{templateWithContext, templateNoContext} {
			var input string
			for _, diff := range test.diffs {
				input += fmt.Sprintf(tmpl, strings.TrimSpace(diff))
			}
			actual := buildPlaybook(readDiffLines(strings.NewReader(input)))
			// the library actions may be in any order, sort them.
			sort.Sort(byLibImportPath(actual.library))
			if !reflect.DeepEqual(actual, test.book) {
				t.Errorf("%v: expected %v, got %v", test.name, test.book, actual)
			}
		}
	}
}

type byLibImportPath []libraryAction

func (b byLibImportPath) Len() int           { return len(b) }
func (b byLibImportPath) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byLibImportPath) Less(i, j int) bool { return b[i].importPath < b[j].importPath }
