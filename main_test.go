package main

import "testing"

func TestPrettyPath(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/a/code/x": "~/code/x",
		"/Users/ab/x":     "/Users/ab/x",
		"/Users/a/code/bg/.claude/worktrees/agent-a0ce567bf89ee92c5/examples/ttt": "~/code/bg/⎇agent-a0ce567/examples/ttt",
	} {
		if got := prettyPath(in, "/Users/a"); got != want {
			t.Errorf("prettyPath(%q) = %q, want %q", in, got, want)
		}
	}
}
