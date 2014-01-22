package main

import (
	"fmt"
	"strings"
	"testing"
)

const TEMPLATE = `25b4da1 change lgo4go to bogus revision
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

var tests = []struct {
	name  string
	input string
	cmds  []command
}{
	{"update existing", fmt.Sprintf(TEMPLATE, `
-code.google.com/p/log4go c3294304d93f
+code.google.com/p/log4go 4794f7baff22
`), []command{
		{update, "code.google.com/p/log4go", "4794f7baff22"},
	}},

	{"update existing 2", fmt.Sprintf(TEMPLATE, `
+code.google.com/p/log4go c3294304d93f
-code.google.com/p/log4go 4794f7baff22
`), []command{
		{update, "code.google.com/p/log4go", "c3294304d93f"},
	}},

	{"add dep", fmt.Sprintf(TEMPLATE, `
+code.google.com/p/log4go 4794f7baff22
`), []command{
		{add, "code.google.com/p/log4go", "4794f7baff22"},
	}},

	{"remove dep", fmt.Sprintf(TEMPLATE, `
-code.google.com/p/log4go 4794f7baff22
`), []command{
		{remove, "code.google.com/p/log4go", "4794f7baff22"},
	}},

	{"no change", "", []command{}},

	{"multiple commits", fmt.Sprintf(TEMPLATE, `
+code.google.com/p/log4go 4794f7baff22
`) + fmt.Sprintf(TEMPLATE, `
+launchpad.net/gocheck 45
`), []command{
		{add, "code.google.com/p/log4go", "4794f7baff22"},
		{add, "launchpad.net/gocheck", "45"},
	}},

	{"multiple commits on same import path", fmt.Sprintf(TEMPLATE, `
-code.google.com/p/log4go 2
+code.google.com/p/log4go 3
`) + fmt.Sprintf(TEMPLATE, `
-code.google.com/p/log4go 1
+code.google.com/p/log4go 2
`), []command{
		{update, "code.google.com/p/log4go", "3"},
	}},
}

func TestLogParser(t *testing.T) {
	for _, test := range tests {
		actual := buildCommands(readDiffLines(strings.NewReader(test.input)))
		if len(actual) != len(test.cmds) {
			t.Errorf("%v: expected %d commands, got %d: %v",
				test.name, len(test.cmds), len(actual), actual)
			continue
		}
		for i, cmd := range actual {
			if cmd != test.cmds[i] {
				t.Errorf("%v: actual %v != %v expected", test.name, cmd, test.cmds[i])
			}
		}
	}
}
