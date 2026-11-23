// Package ups collects "what's running locally" from several sources
// (listening processes, Docker containers, DDEV sites) into one list.
package ups

import (
	"context"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Item is one running thing you can reach: a DDEV site, a container, or a
// group of processes sharing a folder. Its JSON form is what `ups --json`
// prints, so field names are a public interface: add, don't rename.
type Item struct {
	Name    string     `json:"name"`
	Kind    string     `json:"kind"`              // "ddev", "docker", "process" or "claude"
	URL     string     `json:"url,omitempty"`     // the one to open; best of Ports
	Ports   []Port     `json:"ports,omitempty"`   // best first
	Runs    []string   `json:"runs,omitempty"`    // e.g. ["vite", "workerd"], ["ddev wordpress"]
	PIDs    []int      `json:"pids,omitempty"`    // processes only
	Command string     `json:"command,omitempty"` // full command line of the process serving URL
	Image   string     `json:"image,omitempty"`   // docker only
	Started *time.Time `json:"started,omitempty"` // processes only; earliest in the group
	Path    string     `json:"path,omitempty"`    // project folder (or .app for GUI apps)
	Git     *Git       `json:"git,omitempty"`
	Status  string     `json:"status,omitempty"`
	// System: outside $HOME (AirPlay, Raycast, …). Hidden unless --all.
	System bool `json:"system,omitempty"`
	// Internal: infrastructure such as the DDEV router or OrbStack's port
	// proxy. Never listed; only reported by `ups port`.
	Internal bool `json:"internal,omitempty"`

	ddevSite string // docker containers belonging to a DDEV site, merged in Collect
	runtime  bool   // container runtime proxy; lowest priority in ByPort
}

// Port is one listening port of an item.
type Port struct {
	Port int    `json:"port"`
	URL  string `json:"url"`
	// Role: "web", "db", "cache", "mail", "inspector" (Node debugger),
	// "ephemeral" (OS-assigned, usually internal) or "other".
	Role string `json:"role"`
}

// Options controls collection.
type Options struct {
	Home string // processes outside this folder are marked System
}

// Collect runs all collectors concurrently. A source whose CLI is missing or
// fails is skipped, so ups still works without Docker or DDEV installed.
func Collect(ctx context.Context, opt Options) []Item {
	collectors := []func(context.Context, Options) ([]Item, error){
		collectDDEV, collectDocker, collectProcesses,
	}
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		items []Item
	)
	for _, c := range collectors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := c(ctx, opt)
			if err != nil {
				return
			}
			mu.Lock()
			items = append(items, got...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	items = mergeDDEV(items)
	git := gitCache{home: opt.Home, m: map[string]*Git{}}
	for i := range items {
		items[i].Git = git.info(items[i].Path)
	}

	// DDEV and Docker first, then processes; within a kind, by folder so a
	// repo's processes sit together.
	kindOrder := map[string]int{"ddev": 0, "docker": 1, "process": 2, "claude": 3}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Kind != b.Kind {
			return kindOrder[a.Kind] < kindOrder[b.Kind]
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Name < b.Name
	})
	return items
}

// mergeDDEV folds a DDEV site's containers (web, db) into the site's item so
// their host ports (e.g. the MySQL port) belong to the site. Containers whose
// site isn't in the list (DDEV CLI missing) stay as plain docker items.
func mergeDDEV(items []Item) []Item {
	sites := map[string]int{}
	for i, it := range items {
		if it.Kind == "ddev" {
			sites[it.Name] = i
		}
	}
	merged := func(it Item) (int, bool) {
		i, ok := sites[it.ddevSite]
		return i, ok && it.ddevSite != ""
	}
	for _, it := range items {
		if i, ok := merged(it); ok {
			items[i].Ports = append(items[i].Ports, it.Ports...)
		}
	}
	kept := make([]Item, 0, len(items))
	for _, it := range items {
		if _, ok := merged(it); !ok {
			kept = append(kept, it)
		}
	}
	return kept
}

// run executes a CLI with a timeout. It returns stdout even when the exit code
// is non-zero, because lsof exits 1 when some processes can't be inspected.
func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(name); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if len(out) > 0 {
		return out, nil
	}
	return out, err
}

// portOf returns the port from "127.0.0.1:6006", "*:80" or "[::1]:3000".
func portOf(addr string) (int, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0, false
	}
	p, err := strconv.Atoi(addr[i+1:])
	return p, err == nil
}
