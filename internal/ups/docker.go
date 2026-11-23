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

// Scheme and role for well-known container ports, so a Postgres port isn't
// shown as http://.
var containerPorts = map[int]struct{ scheme, role string }{
	80: {"http", "web"}, 8080: {"http", "web"}, 3000: {"http", "web"},
	5173: {"http", "web"}, 8000: {"http", "web"},
	443: {"https", "web"}, 8443: {"https", "web"},
	5432: {"postgres", "db"}, 3306: {"mysql", "db"}, 27017: {"mongodb", "db"},
	6379: {"redis", "cache"},
	8025: {"http", "mail"}, 8026: {"https", "mail"}, // Mailpit
}

type dockerPS struct {
	Names  string
	Image  string
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
		name, runs := c.Names, "docker"
		if p, s := labels["com.docker.compose.project"], labels["com.docker.compose.service"]; p != "" && s != "" {
			name, runs = p, "docker "+s
		}
		it := Item{
			Name:   name,
			Kind:   "docker",
			Runs:   []string{runs},
			Ports:  dockerPorts(c.Ports),
			Image:  c.Image,
			Path:   labels["com.docker.compose.project.working_dir"],
			Status: c.Status,
		}
		it.URL = bestPortURL(it.Ports)
		// DDEV site containers get merged into the DDEV site (see mergeDDEV);
		// the shared ones (router, ssh-agent) are infrastructure.
		if labels["com.ddev.platform"] == "ddev" {
			if site := labels["com.ddev.site-name"]; site != "" {
				it.ddevSite = site
			} else {
				it.Internal = true
			}
		}
		items = append(items, it)
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

// dockerPorts turns "127.0.0.1:8025-8026->8025-8026/tcp, :::5432->5432/tcp"
// into one Port per published host port. Unpublished ports are skipped.
func dockerPorts(ports string) []Port {
	var out []Port
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
			port := Port{Port: p, URL: fmt.Sprintf("localhost:%d", p), Role: "other"}
			if known, ok := containerPorts[cFrom+(p-hostFrom)]; ok {
				port.URL = fmt.Sprintf("%s://localhost:%d", known.scheme, p)
				port.Role = known.role
			}
			out = append(out, port)
		}
	}
	return out
}

// bestPortURL is the first web port's URL, else the first port's.
func bestPortURL(ports []Port) string {
	for _, p := range ports {
		if p.Role == "web" {
			return p.URL
		}
	}
	if len(ports) > 0 {
		return ports[0].URL
	}
	return ""
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
