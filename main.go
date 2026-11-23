// Command ups lists running local projects: DDEV sites, Docker containers,
// and processes listening on TCP ports, with their URLs and folders.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/xavortm/ups/internal/ups"
)

const usage = `ups — what's up and running locally

Usage:
  ups               list running projects
  ups <path>        only the project at path, e.g. "ups ." (its git repo; worktrees separate)
  ups url [path]    print the best URL for the project at path (default ".")
  ups port <n>      show what is listening on port n

Flags (anywhere on the line):
  -a, --all         include system/app listeners (AirPlay, Raycast, …)
      --json        JSON output: name, kind, url, ports[{port,url,role}], runs, pids,
                    command, image, started, path, git{root,branch,worktree,main_root}
  -q, --quiet       no output, exit status only

Exit status: 0 found, 1 nothing running / port free, 2 usage error.

Agents: "ups . --json" before starting a dev server; "ups port <n>" on "address in use".
`

// Exit codes.
const (
	exitFound    = 0
	exitNotFound = 1
	exitUsage    = 2
)

type options struct {
	all, json, quiet bool
}

func main() {
	os.Exit(cli(os.Args[1:], os.Stdout, os.Stderr))
}

func cli(args []string, stdout, stderr io.Writer) int {
	opt, pos, err := parseArgs(args)
	if err == flag.ErrHelp {
		fmt.Fprint(stdout, usage)
		return exitFound
	}
	if err != nil {
		fmt.Fprintf(stderr, "ups: %v\n\n%s", err, usage)
		return exitUsage
	}
	if opt.quiet {
		stdout = io.Discard
	}
	home, _ := os.UserHomeDir()

	switch {
	case len(pos) > 0 && pos[0] == "port":
		if len(pos) != 2 {
			fmt.Fprintf(stderr, "ups: usage: ups port <number>\n")
			return exitUsage
		}
		port, err := strconv.Atoi(pos[1])
		if err != nil || port < 1 || port > 65535 {
			fmt.Fprintf(stderr, "ups: invalid port %q\n", pos[1])
			return exitUsage
		}
		return cmdPort(port, opt, home, stdout)

	case len(pos) > 0 && pos[0] == "url":
		if len(pos) > 2 {
			fmt.Fprintf(stderr, "ups: usage: ups url [path]\n")
			return exitUsage
		}
		dir := "."
		if len(pos) == 2 {
			dir = pos[1]
		}
		return cmdURL(dir, opt, home, stdout, stderr)

	case len(pos) > 1:
		fmt.Fprintf(stderr, "ups: too many arguments: %s\n", strings.Join(pos, " "))
		return exitUsage
	}

	dir := ""
	if len(pos) == 1 {
		dir = pos[0]
	}
	return cmdList(dir, opt, home, stdout, stderr)
}

// parseArgs accepts flags before or after positional arguments
// ("ups . --json" and "ups --json ." both work), which the flag package
// alone doesn't do.
func parseArgs(args []string) (options, []string, error) {
	var opt options
	fs := flag.NewFlagSet("ups", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opt.all, "all", false, "")
	fs.BoolVar(&opt.all, "a", false, "")
	fs.BoolVar(&opt.json, "json", false, "")
	fs.BoolVar(&opt.quiet, "quiet", false, "")
	fs.BoolVar(&opt.quiet, "q", false, "")

	var flags, pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
		} else {
			pos = append(pos, a)
		}
	}
	return opt, pos, fs.Parse(flags)
}

func collect(home string) []ups.Item {
	return ups.Collect(context.Background(), ups.Options{Home: home})
}

// scoped resolves dir and returns the items of that project, or all items
// when dir is empty.
func scoped(items []ups.Item, dir, home string) ([]ups.Item, string, error) {
	if dir == "" {
		return items, "", nil
	}
	abs, err := ups.Clean(dir)
	if err != nil {
		return nil, "", err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return nil, "", fmt.Errorf("not a folder: %s", dir)
	}
	root, _ := ups.ProjectRoot(abs, home)
	return ups.Scope(items, abs, home), root, nil
}

func cmdList(dir string, opt options, home string, stdout, stderr io.Writer) int {
	items, root, err := scoped(collect(home), dir, home)
	if err != nil {
		fmt.Fprintf(stderr, "ups: %v\n", err)
		return exitUsage
	}

	shown := []ups.Item{} // JSON: [] rather than null when empty
	hidden, found := 0, false
	for _, it := range items {
		if it.Internal {
			continue
		}
		if !ups.Listed(it, opt.all) {
			hidden++
			continue
		}
		shown = append(shown, it)
		found = found || it.Kind != "claude"
	}

	if opt.json {
		writeJSON(stdout, shown)
	} else {
		printTable(stdout, shown, hidden, home, root)
	}
	if found {
		return exitFound
	}
	return exitNotFound
}

func cmdURL(dir string, opt options, home string, stdout, stderr io.Writer) int {
	items, root, err := scoped(collect(home), dir, home)
	if err != nil {
		fmt.Fprintf(stderr, "ups: %v\n", err)
		return exitUsage
	}
	abs, _ := ups.Clean(dir)
	u := ups.BestURL(items, abs)
	if u == "" {
		if !opt.quiet {
			fmt.Fprintf(stderr, "ups: no web URL running for %s\n", prettyPath(root, home))
		}
		return exitNotFound
	}
	if opt.json {
		writeJSON(stdout, map[string]string{"url": u})
	} else {
		fmt.Fprintln(stdout, u)
	}
	return exitFound
}

func cmdPort(port int, opt options, home string, stdout io.Writer) int {
	items := collect(home)
	matches := ups.ByPort(items, port) // includes system & internal on purpose
	var sites []string
	for _, it := range items {
		if it.Kind == "ddev" {
			sites = append(sites, it.Name)
		}
	}
	if opt.json {
		if matches == nil {
			matches = []ups.Item{}
		}
		writeJSON(stdout, matches)
	} else if len(matches) == 0 {
		fmt.Fprintf(stdout, "Port %d is free.\n", port)
	} else {
		for i, it := range matches {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			printDetail(stdout, it, port, home, sites)
		}
	}
	if len(matches) == 0 {
		return exitNotFound
	}
	return exitFound
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false) // keep "->" and "&" readable in commands
	_ = enc.Encode(v)
}

func printTable(w io.Writer, items []ups.Item, hidden int, home, root string) {
	var rows, claude []ups.Item
	for _, it := range items {
		if it.Kind == "claude" {
			claude = append(claude, it)
		} else {
			rows = append(rows, it)
		}
	}

	if len(rows) == 0 {
		if root != "" {
			fmt.Fprintf(w, "Nothing running in %s.\n", prettyPath(root, home))
		} else {
			fmt.Fprintln(w, "Nothing running.")
		}
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tURL\t\tRUNS\tFOLDER\tPID")
		for _, it := range rows {
			url, more := it.URL, ""
			if url == "" {
				url = "-"
			}
			if n := extraPorts(it); n > 0 {
				more = fmt.Sprintf("+%d", n)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				it.Name, url, more, strings.Join(it.Runs, ", "), prettyPath(it.Path, home), pids(it.PIDs))
		}
		tw.Flush()
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
		fmt.Fprintln(w)
		for _, n := range notes {
			fmt.Fprintln(w, n)
		}
	}
}

// extraPorts counts ports beyond the one shown in the URL column.
func extraPorts(it ups.Item) int {
	n := 0
	for _, p := range it.Ports {
		if p.URL != it.URL {
			n++
		}
	}
	return n
}

// printDetail is the `ups port` view: everything known about one item.
func printDetail(w io.Writer, it ups.Item, port int, home string, ddevSites []string) {
	role, portURL := "", ""
	for _, p := range it.Ports {
		if p.Port == port {
			portURL = p.URL
			if p.Role != "web" {
				role = " (" + p.Role + " port)"
			}
		}
	}
	url := it.URL
	if it.Internal {
		url = portURL // infra has no "main" URL; show the one asked about
	}
	fmt.Fprintf(w, ":%d%s → %s\n", port, role, it.Name)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	field := func(k, v string) {
		if v != "" {
			fmt.Fprintf(tw, "  %s\t%s\n", k, v)
		}
	}
	kind := it.Kind
	if it.Internal {
		kind += ", infrastructure"
	}
	field("kind", kind)
	field("url", url)
	if it.Internal && it.Name == "ddev-router" && len(ddevSites) > 0 {
		field("serves", "DDEV sites: "+strings.Join(ddevSites, ", "))
	}
	field("runs", strings.Join(it.Runs, ", "))
	field("pid", joinInts(it.PIDs))
	field("folder", prettyPath(it.Path, home))
	if g := it.Git; g != nil {
		v := prettyPath(g.Root, home) + " @ " + g.Branch
		if g.Worktree {
			v += " (worktree of " + prettyPath(g.MainRoot, home) + ")"
		}
		field("git", v)
	}
	if it.Started != nil {
		field("started", it.Started.Format("2006-01-02 15:04")+" ("+ago(time.Since(*it.Started))+" ago)")
	}
	field("status", it.Status)
	field("image", it.Image)
	field("command", strings.ReplaceAll(it.Command, home+"/", "~/"))
	tw.Flush()
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func joinInts(ns []int) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ", ")
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
