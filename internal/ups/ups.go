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

// Item is one running thing you can reach: a DDEV site, a container, or a process.
type Item struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`           // "ddev", "docker", "process" or "claude"
	URLs   []string `json:"urls"`           // best one first
	Runs   []string `json:"runs,omitempty"` // e.g. ["vite", "workerd"], ["ddev wordpress"]
	PIDs   []int    `json:"pids,omitempty"`
	Path   string   `json:"path,omitempty"`
	Status string   `json:"status,omitempty"`
	System bool     `json:"system,omitempty"` // outside $HOME; hidden unless --all

	ports []int // processes only, used while merging
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
