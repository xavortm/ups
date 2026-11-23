// Command ups lists running local projects: DDEV sites, Docker containers,
// and processes listening on TCP ports, with their URLs and folders.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/xavortm/ups/internal/ups"
)

func main() {
	var all, asJSON bool
	flag.BoolVar(&all, "all", false, "include system/app listeners outside your home folder")
	flag.BoolVar(&all, "a", false, "shorthand for --all")
	flag.BoolVar(&asJSON, "json", false, "print JSON (all URLs and PIDs) instead of a table")
	flag.Parse()

	home, _ := os.UserHomeDir()
	items := ups.Collect(context.Background(), ups.Options{Home: home})

	var shown []ups.Item
	hidden := 0
	for _, it := range items {
		if it.System && !all {
			hidden++
			continue
		}
		shown = append(shown, it)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(shown); err != nil {
			fmt.Fprintln(os.Stderr, "ups:", err)
			os.Exit(1)
		}
		return
	}
	printTable(shown, hidden, home)
}

func printTable(items []ups.Item, hidden int, home string) {
	var rows, claude []ups.Item
	for _, it := range items {
		if it.Kind == "claude" {
			claude = append(claude, it)
		} else {
			rows = append(rows, it)
		}
	}

	if len(rows) == 0 {
		fmt.Println("Nothing running.")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tURL\t\tRUNS\tFOLDER\tPID")
		for _, it := range rows {
			url, more := "-", ""
			if len(it.URLs) > 0 {
				url = it.URLs[0]
			}
			if n := len(it.URLs) - 1; n > 0 {
				more = fmt.Sprintf("+%d", n)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				it.Name, url, more, strings.Join(it.Runs, ", "), prettyPath(it.Path, home), pids(it.PIDs))
		}
		w.Flush()
	}

	var notes []string
	if len(claude) > 0 {
		names := make([]string, len(claude))
		for i, c := range claude {
			names[i] = c.Name
		}
		notes = append(notes, "Claude Code sessions: "+strings.Join(names, ", "))
	}
	if hidden > 0 {
		notes = append(notes, fmt.Sprintf("%d system/app listeners hidden (ups -a to show)", hidden))
	}
	if len(notes) > 0 {
		fmt.Println()
		for _, n := range notes {
			fmt.Println(n)
		}
	}
}

// pids shows the first PID and how many more are merged into the row.
func pids(ps []int) string {
	switch len(ps) {
	case 0:
		return ""
	case 1:
		return strconv.Itoa(ps[0])
	}
	return fmt.Sprintf("%d +%d", ps[0], len(ps)-1)
}

var worktreeRe = regexp.MustCompile(`/\.claude/worktrees/([^/]+)`)

// prettyPath shortens $HOME to ~ and ".claude/worktrees/agent-a0ce567bf89…" to "⎇agent-a0ce567".
func prettyPath(p, home string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		p = "~" + strings.TrimPrefix(p, home)
	}
	return worktreeRe.ReplaceAllStringFunc(p, func(m string) string {
		name := worktreeRe.FindStringSubmatch(m)[1]
		if len(name) > 13 {
			name = name[:13]
		}
		return "/⎇" + name
	})
}
