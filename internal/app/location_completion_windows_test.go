package app

import "testing"

func TestWindowsLocationCompletionParents(t *testing.T) {
	for _, tc := range []struct{ query, parent, prefix string }{
		{`C:\Wor`, `C:\`, `C:\`},
		{`C:\Work\pro`, `C:\Work`, `C:\Work\`},
		{`\Work\pro`, `C:\Work`, `\Work\`},
		{`..\pro`, `C:\`, `..\`},
		{`C:`, `C:\`, `C:\`},
		{`D:pro`, `D:.`, `D:`},
		{`.\d`, `C:\Work`, `.\`},
		{`"C:\Work\pro"`, `C:\Work`, `"C:\Work\`},
	} {
		plan, ok := completionLocation(`C:\Work`, tc.query)
		if !ok || plan.directory != tc.parent || plan.prefix != tc.prefix {
			t.Fatalf("%q: plan=%+v ok=%v", tc.query, plan, ok)
		}
	}
	t.Setenv("COMMANDO_REMOTE_COMPLETION", `\\server\share`)
	if _, ok := completionLocation(`C:\Work`, `%COMMANDO_REMOTE_COMPLETION%\pro`); ok {
		t.Fatal("expanded network path offered completion")
	}
}
