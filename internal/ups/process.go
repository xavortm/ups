package ups

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Name prefixes of processes that only proxy ports for containers
// ("OrbStack Helper", "com.docker.backend"); the Docker collector covers them.
var containerRuntimes = []string{"OrbStack", "com.docker.", "vpnkit"}

func isContainerRuntime(cmd string) bool {
	for _, p := range containerRuntimes {
		if strings.HasPrefix(cmd, p) {
			return true
		}
	}
	return false
}

// Dev tools recognised in a process's command line, most specific first
// (storybook and vite both pull in esbuild, so order matters).
var tools = []string{
	"storybook", "vite", "next", "astro", "nuxt", "remix", "react-router",
	"wrangler", "workerd", "webpack", "parcel", "turbo", "nodemon", "tsx",
	"hugo", "uvicorn", "gunicorn", "flask", "rails",
}

var toolRes = func() []*regexp.Regexp {
	rs := make([]*regexp.Regexp, len(tools))
	for i, t := range tools {
		// A path segment or word: ".../node_modules/vite/bin", "vite@8.3", "next dev".
		rs[i] = regexp.MustCompile(`(^|[/\s])` + regexp.QuoteMeta(t) + `([/@\s.]|$)`)
	}
	return rs
}()

type listener struct {
	pid   int
	cmd   string
	ports []int
}

func collectProcesses(ctx context.Context, opt Options) ([]Item, error) {
	out, err := run(ctx, "lsof", "-nP", "+c", "0", "-iTCP", "-sTCP:LISTEN", "-F", "pcn")
	if err != nil {
		return nil, err
	}
	ls := parseLsofListen(out)
	if len(ls) == 0 {
		return nil, nil
	}

	pids := make([]string, len(ls))
	for i, l := range ls {
		pids[i] = strconv.Itoa(l.pid)
	}
	pidList := strings.Join(pids, ",")
	cwdOut, _ := run(ctx, "lsof", "-a", "-d", "cwd", "-F", "pn", "-p", pidList)
	commOut, _ := run(ctx, "ps", "-o", "pid=,comm=", "-p", pidList)
	argsOut, _ := run(ctx, "ps", "-o", "pid=,args=", "-p", pidList)
	startOut, _ := run(ctx, "ps", "-o", "pid=,lstart=", "-p", pidList)

	procs := make([]proc, len(ls))
	cwds, exes, args := parseLsofCwd(cwdOut), parsePs(commOut), parsePs(argsOut)
	starts := parsePs(startOut)
	for i, l := range ls {
		procs[i] = proc{listener: l, cwd: cwds[l.pid], exe: exes[l.pid], args: args[l.pid]}
		// lstart is local time, e.g. "Mon Nov 23 18:45:33 2026".
		if t, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", starts[l.pid], time.Local); err == nil {
			procs[i].started = &t
		}
	}
	return groupProcesses(procs, opt.Home), nil
}

type proc struct {
	listener
	cwd, exe, args string
	started        *time.Time
}

// groupProcesses turns listeners into items, merging processes that share a
// project folder (e.g. vite + its workerd child) into one item.
func groupProcesses(procs []proc, home string) []Item {
	type group struct {
		item    Item
		ports   []int
		portPID map[int]int    // which process owns each port
		args    map[int]string // pid -> command line
	}
	var (
		groups []*group
		byKey  = map[string]*group{}
	)
	for _, p := range procs {
		kind := "process"
		if p.cmd == "claude" || filepath.Base(p.exe) == "claude" {
			kind = "claude"
		}
		runtime := isContainerRuntime(p.cmd)
		system := !under(p.cwd, home) && !under(p.exe, home)

		name, path := p.cmd, p.cwd
		switch {
		case under(p.cwd, home):
			name = filepath.Base(p.cwd)
		case appBundle(p.exe) != "":
			// GUI apps run from "/"; the .app is the useful location.
			path = appBundle(p.exe)
			name = strings.TrimSuffix(filepath.Base(path), ".app")
		}

		key := kind + "\x00" + path
		if system || runtime || path == "" || path == "/" {
			key = "pid\x00" + strconv.Itoa(p.pid) // never merge unrelated system processes
		}
		g := byKey[key]
		if g == nil {
			g = &group{
				item:    Item{Name: name, Kind: kind, Path: path, System: system, Internal: runtime, runtime: runtime},
				portPID: map[int]int{},
				args:    map[int]string{},
			}
			byKey[key] = g
			groups = append(groups, g)
		}

		it := &g.item
		it.PIDs = append(it.PIDs, p.pid)
		runs := detectTool(p.args, p.exe, p.cmd)
		if runtime {
			runs = "container runtime"
		}
		if !contains(it.Runs, runs) {
			it.Runs = append(it.Runs, runs)
		}
		if p.started != nil && (it.Started == nil || p.started.Before(*it.Started)) {
			it.Started = p.started
		}
		g.args[p.pid] = p.args
		for _, port := range p.ports {
			if _, ok := g.portPID[port]; !ok {
				g.portPID[port] = p.pid
				g.ports = append(g.ports, port)
			}
		}
	}

	items := make([]Item, len(groups))
	for i, g := range groups {
		sort.SliceStable(g.ports, func(a, b int) bool {
			ra, rb := portRank(g.ports[a]), portRank(g.ports[b])
			if ra != rb {
				return ra < rb
			}
			return g.ports[a] < g.ports[b]
		})
		it := g.item
		for _, port := range g.ports {
			it.Ports = append(it.Ports, Port{Port: port, URL: fmt.Sprintf("http://localhost:%d", port), Role: portRole(port)})
		}
		if len(g.ports) > 0 {
			it.URL = it.Ports[0].URL
			it.Command = g.args[g.portPID[g.ports[0]]] // the process serving URL
		}
		items[i] = it
	}
	return items
}

// portRank orders a process's ports so the one you'd open in a browser comes
// first: well-known dev-server ports, then ordinary ports, then Node
// inspector ports, then OS-assigned ephemeral ports.
func portRank(p int) int {
	switch {
	case p == 1313, p == 4000, p == 4173, p == 4200, p == 4321, p == 6006,
		p >= 3000 && p <= 3010, p >= 5173 && p <= 5199, p >= 8000 && p <= 8100,
		p == 8787, p == 8888:
		return 0
	case p >= 9229 && p <= 9239:
		return 2
	case p >= 49152:
		return 3
	}
	return 1
}

func portRole(p int) string {
	return [...]string{"web", "other", "inspector", "ephemeral"}[portRank(p)]
}

// detectTool names what a process is running: a known dev tool from its
// command line, else the executable's name.
func detectTool(args, exe, cmd string) string {
	for i, re := range toolRes {
		if re.MatchString(args) {
			return tools[i]
		}
	}
	if exe != "" {
		return filepath.Base(exe)
	}
	return cmd
}

// appBundle returns "/Applications/Foo.app" for an executable inside it.
func appBundle(exe string) string {
	if i := strings.Index(exe, ".app/"); i >= 0 {
		return exe[:i+len(".app")]
	}
	return ""
}

// under reports whether path is strictly inside dir (dir itself doesn't count).
func under(path, dir string) bool {
	return dir != "" && strings.HasPrefix(path, strings.TrimSuffix(dir, "/")+"/")
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// parseLsofListen parses `lsof -F pcn` output: one "p<pid>" line per process,
// then "c<command>", then "f<fd>"/"n<address>" pairs per socket.
func parseLsofListen(out []byte) []listener {
	var (
		ls   []listener
		cur  *listener
		seen map[int]bool
	)
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, err := strconv.Atoi(line[1:])
			if err != nil {
				cur = nil
				continue
			}
			ls = append(ls, listener{pid: pid})
			cur, seen = &ls[len(ls)-1], map[int]bool{}
		case 'c':
			if cur != nil {
				cur.cmd = line[1:]
			}
		case 'n':
			// IPv4 and IPv6 sockets on the same port show up twice; keep one.
			if p, ok := portOf(line[1:]); ok && cur != nil && !seen[p] {
				seen[p] = true
				cur.ports = append(cur.ports, p)
			}
		}
	}
	return ls
}

// parseLsofCwd parses `lsof -d cwd -F pn` into pid -> working directory.
func parseLsofCwd(out []byte) map[int]string {
	m := map[int]string{}
	pid := 0
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'n':
			if pid != 0 {
				m[pid] = line[1:]
			}
		}
	}
	return m
}

// parsePs parses `ps -o pid=,<field>=` into pid -> field (which may contain spaces).
func parsePs(out []byte) map[int]string {
	m := map[int]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // args lines get long
	for sc.Scan() {
		pidStr, rest, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		if pid, err := strconv.Atoi(pidStr); err == nil {
			m[pid] = strings.TrimSpace(rest)
		}
	}
	return m
}
