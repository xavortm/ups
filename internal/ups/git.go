package ups

import (
	"os"
	"path/filepath"
	"strings"
)

// Git describes the repository an item's folder belongs to.
type Git struct {
	Root     string `json:"root"`                // repo (or worktree) top-level folder
	Branch   string `json:"branch,omitempty"`    // "main", or "detached@abc1234"
	Worktree bool   `json:"worktree,omitempty"`  // a linked worktree, not the main checkout
	MainRoot string `json:"main_root,omitempty"` // worktrees only: the main checkout
}

// gitCache reads .git from disk (no `git` subprocess) and caches by folder.
type gitCache struct {
	home string // never treat $HOME itself as a repo root (dotfile repos)
	m    map[string]*Git
}

func (c *gitCache) info(dir string) *Git {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil
	}
	if g, ok := c.m[dir]; ok {
		return g
	}
	var g *Git
	for d := dir; ; d = filepath.Dir(d) {
		if d == c.home || d == "/" {
			break
		}
		if g = readGit(d); g != nil {
			break
		}
	}
	c.m[dir] = g
	return g
}

// readGit returns repo info if dir contains .git (a folder for a normal
// checkout, a "gitdir: …" file for worktrees and submodules).
func readGit(dir string) *Git {
	dotGit := filepath.Join(dir, ".git")
	fi, err := os.Stat(dotGit)
	if err != nil {
		return nil
	}
	g := &Git{Root: dir}
	gitDir := dotGit
	if !fi.IsDir() {
		b, err := os.ReadFile(dotGit)
		if err != nil {
			return nil
		}
		gd, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return nil
		}
		if !filepath.IsAbs(gd) {
			gd = filepath.Join(dir, gd)
		}
		gitDir = filepath.Clean(gd)
		// Worktrees: <main>/.git/worktrees/<name>. Submodules use .git/modules.
		if i := strings.Index(gitDir, string(filepath.Separator)+filepath.Join(".git", "worktrees")+string(filepath.Separator)); i >= 0 {
			g.Worktree, g.MainRoot = true, gitDir[:i]
		}
	}
	if b, err := os.ReadFile(filepath.Join(gitDir, "HEAD")); err == nil {
		head := strings.TrimSpace(string(b))
		if ref, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
			g.Branch = ref
		} else if len(head) >= 7 {
			g.Branch = "detached@" + head[:7]
		}
	}
	return g
}
