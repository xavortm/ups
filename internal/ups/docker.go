package ups

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Schemes for well-known container ports, so a Postgres port isn't shown as http://.
var schemes = map[int]string{
	80: "http", 8080: "http", 3000: "http", 5173: "http", 8000: "http",
	443: "https", 8443: "https",
	5432: "postgres", 3306: "mysql", 6379: "redis", 27017: "mongodb",
}

type dockerPS struct {
	Names  string
	Ports  string
	Labels string
	Status string
}

func collectDocker(ctx context.Context, _ Options) ([]Item, error) {
	out, err := run(ctx, "docker", "ps", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseDockerPS(out), nil
}

func parseDockerPS(out []byte) []Item {
	var items []Item
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // label lines get long
	for sc.Scan() {
		var c dockerPS
		if json.Unmarshal(sc.Bytes(), &c) != nil {
			continue
		}
		labels := parseLabels(c.Labels)
		// DDEV containers are reported by the DDEV collector as one site.
		if labels["com.ddev.platform"] == "ddev" {
			continue
		}
		name, runs := c.Names, "docker"
		if p, s := labels["com.docker.compose.project"], labels["com.docker.compose.service"]; p != "" && s != "" {
			name, runs = p, "docker "+s
		}
		items = append(items, Item{
			Name:   name,
			Kind:   "docker",
			Runs:   []string{runs},
			URLs:   dockerURLs(c.Ports),
			Path:   labels["com.docker.compose.project.working_dir"],
			Status: c.Status,
		})
	}
	return items
}

// parseLabels parses "k=v,k2=v2". Values can contain commas, so a segment
// without "=" is glued back onto the previous value.
func parseLabels(s string) map[string]string {
	m := map[string]string{}
	last := ""
	for _, seg := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(seg, "="); ok {
			m[k], last = v, k
		} else if last != "" {
			m[last] += "," + seg
		}
	}
	return m
}

// dockerURLs turns "127.0.0.1:8025-8026->8025-8026/tcp, :::5432->5432/tcp"
// into one address per published host port. Unpublished ports are skipped.
func dockerURLs(ports string) []string {
	var urls []string
	seen := map[int]bool{}
	for _, mapping := range strings.Split(ports, ", ") {
		host, container, ok := strings.Cut(mapping, "->")
		if !ok {
			continue
		}
		hostFrom, hostTo, ok := portRange(host[strings.LastIndex(host, ":")+1:])
		if !ok {
			continue
		}
		cFrom, _, _ := portRange(strings.SplitN(container, "/", 2)[0])
		for p := hostFrom; p <= hostTo; p++ {
			if seen[p] {
				continue
			}
			seen[p] = true
			if scheme := schemes[cFrom+(p-hostFrom)]; scheme != "" {
				urls = append(urls, fmt.Sprintf("%s://localhost:%d", scheme, p))
			} else {
				urls = append(urls, fmt.Sprintf("localhost:%d", p))
			}
		}
	}
	return urls
}

// portRange parses "8025" or "8025-8026".
func portRange(s string) (from, to int, ok bool) {
	a, b, isRange := strings.Cut(s, "-")
	from, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, false
	}
	if !isRange {
		return from, from, true
	}
	to, err = strconv.Atoi(b)
	if err != nil || to < from {
		return 0, 0, false
	}
	return from, to, true
}
