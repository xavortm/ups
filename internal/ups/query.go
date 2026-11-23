package ups

import (
	"path/filepath"
	"strings"
)

// ProjectRoot returns the folder that defines "this project" for dir: its git
// top-level (a worktree is its own project), or dir itself outside git.
func ProjectRoot(dir, home string) (root string, g *Git) {
	c := gitCache{home: home, m: map[string]*Git{}}
	if g = c.info(dir); g != nil {
		return g.Root, g
	}
	return dir, nil
}

// Scope keeps the items belonging to the project at dir. With git on both
// sides it compares repo roots, so the main checkout and its worktrees stay
// separate even though worktrees live inside the main folder.
func Scope(items []Item, dir, home string) []Item {
	root, g := ProjectRoot(dir, home)
	var out []Item
	for _, it := range items {
		var in bool
		if g != nil && it.Git != nil {
			in = it.Git.Root == g.Root
		} else {
			in = it.Path == root || under(it.Path, root)
		}
		if in {
			out = append(out, it)
		}
	}
	return out
}

// ByPort returns what listens on port. Container-runtime proxies (OrbStack)
// are only returned when nothing more specific (the container) matches.
func ByPort(items []Item, port int) []Item {
	var specific, runtimes []Item
	for _, it := range items {
		for _, p := range it.Ports {
			if p.Port != port {
				continue
			}
			if it.runtime {
				runtimes = append(runtimes, it)
			} else {
				specific = append(specific, it)
			}
			break
		}
	}
	if len(specific) > 0 {
		return specific
	}
	return runtimes
}

// BestURL picks the URL you'd open for a project. Candidates are DDEV sites
// and items whose best port is a web port. Preference, in order:
//  1. closeness to dir: an item running in dir (or a parent of it), then one
//     in a subfolder of dir (shallowest first), then anything else in scope;
//  2. kind: DDEV site, then dev-server process, then container.
//
// Empty if there's no candidate. dir may be "" to skip the closeness rule.
func BestURL(items []Item, dir string) string {
	kindRank := map[string]int{"ddev": 0, "process": 1, "docker": 2}
	best, bestKey := "", [3]int{}
	for _, it := range items {
		kr, ok := kindRank[it.Kind]
		if !ok || it.Internal || it.URL == "" {
			continue
		}
		if it.Kind != "ddev" && (len(it.Ports) == 0 || it.Ports[0].Role != "web") {
			continue
		}
		tier := 2
		switch {
		case dir != "" && (it.Path == dir || under(dir, it.Path)):
			tier = 0
		case dir != "" && under(it.Path, dir):
			tier = 1
		}
		key := [3]int{tier, kr, strings.Count(it.Path, "/")}
		if best == "" || less(key, bestKey) {
			best, bestKey = it.URL, key
		}
	}
	return best
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// Listed reports whether an item belongs in list output (not infra, and not a
// system listener unless all is set).
func Listed(it Item, all bool) bool {
	return !it.Internal && (all || !it.System)
}

// Clean resolves a user-supplied folder (".", "../x", symlinks like /tmp).
func Clean(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	return abs, nil
}
