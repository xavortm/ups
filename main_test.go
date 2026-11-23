package main

import (
	"strings"
	"testing"
)

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

func TestParseArgs(t *testing.T) {
	opt, pos, err := parseArgs([]string{".", "--json", "-a"})
	if err != nil || !opt.json || !opt.all || opt.quiet || len(pos) != 1 || pos[0] != "." {
		t.Fatalf("opt=%+v pos=%v err=%v", opt, pos, err)
	}
	if _, _, err := parseArgs([]string{"--nope"}); err == nil {
		t.Error("unknown flag should error")
	}
}

func TestCLIUsageErrors(t *testing.T) {
	var out, errb strings.Builder
	for _, args := range [][]string{
		{"port"}, {"port", "abc"}, {"port", "70000"}, {"url", "a", "b"}, {"a", "b"}, {"--nope"},
		{"/definitely/not/a/folder"},
	} {
		if code := cli(args, &out, &errb); code != exitUsage {
			t.Errorf("cli(%v) = %d, want %d", args, code, exitUsage)
		}
	}
	if code := cli([]string{"-h"}, &out, &errb); code != exitFound || !strings.Contains(out.String(), "Usage:") {
		t.Errorf("-h: code %d, out %q", code, out.String())
	}
}
