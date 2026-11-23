package ups

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// makeRepo creates <root>/main as a git checkout on branch "main" with a
// linked worktree at <root>/main/.claude/worktrees/wt on branch "feature".
func makeRepo(t *testing.T) (root, main, wt string) {
	t.Helper()
	root, _ = filepath.EvalSymlinks(t.TempDir())
	main = filepath.Join(root, "main")
	wt = filepath.Join(main, ".claude", "worktrees", "wt")
	wtGitDir := filepath.Join(main, ".git", "worktrees", "wt")
	for _, d := range []string{filepath.Join(main, "pkg"), wt, wtGitDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
	write(filepath.Join(wtGitDir, "HEAD"), "ref: refs/heads/feature\n")
	write(filepath.Join(wt, ".git"), "gitdir: "+wtGitDir+"\n")
	return root, main, wt
}

func TestGitInfo(t *testing.T) {
	root, main, wt := makeRepo(t)
	c := gitCache{home: root, m: map[string]*Git{}}

	if g := c.info(filepath.Join(main, "pkg")); !reflect.DeepEqual(g, &Git{Root: main, Branch: "main"}) {
		t.Errorf("main = %+v", g)
	}
	if g := c.info(wt); !reflect.DeepEqual(g, &Git{Root: wt, Branch: "feature", Worktree: true, MainRoot: main}) {
		t.Errorf("worktree = %+v", g)
	}
	if g := c.info(root); g != nil {
		t.Errorf("outside git = %+v", g)
	}
}

func TestScopeSeparatesWorktrees(t *testing.T) {
	root, main, wt := makeRepo(t)
	c := gitCache{home: root, m: map[string]*Git{}}
	items := []Item{
		{Name: "api", Path: filepath.Join(main, "pkg")},
		{Name: "wt-server", Path: wt},
		{Name: "other", Path: filepath.Join(root, "elsewhere")},
	}
	for i := range items {
		items[i].Git = c.info(items[i].Path)
	}
	names := func(its []Item) (out []string) {
		for _, it := range its {
			out = append(out, it.Name)
		}
		return out
	}
	if got := names(Scope(items, main, root)); !reflect.DeepEqual(got, []string{"api"}) {
		t.Errorf("scope main = %v", got)
	}
	if got := names(Scope(items, wt, root)); !reflect.DeepEqual(got, []string{"wt-server"}) {
		t.Errorf("scope worktree = %v", got)
	}
	// Outside git: everything under the folder.
	if got := names(Scope(items, root, "")); len(got) != 3 {
		t.Errorf("scope root = %v", got)
	}
}

func TestByPortAndBestURL(t *testing.T) {
	items := []Item{
		{Name: "OrbStack", Kind: "process", Internal: true, runtime: true, Ports: []Port{{80, "", "web"}, {32222, "", "other"}}},
		{Name: "ddev-router", Kind: "docker", Internal: true, Ports: []Port{{80, "http://localhost:80", "web"}}},
		{Name: "db", Kind: "docker", URL: "postgres://localhost:5432", Ports: []Port{{5432, "postgres://localhost:5432", "db"}}},
		{Name: "app", Kind: "process", URL: "http://localhost:5173", Ports: []Port{{5173, "http://localhost:5173", "web"}}},
	}
	if got := ByPort(items, 80); len(got) != 1 || got[0].Name != "ddev-router" {
		t.Errorf("port 80 = %+v (container should win over runtime proxy)", got)
	}
	if got := ByPort(items, 32222); len(got) != 1 || got[0].Name != "OrbStack" {
		t.Errorf("port 32222 = %+v", got)
	}
	if got := ByPort(items, 9999); len(got) != 0 {
		t.Errorf("port 9999 = %+v", got)
	}
	if got := BestURL(items, ""); got != "http://localhost:5173" {
		t.Errorf("BestURL = %q (dev server should beat a db)", got)
	}
	if got := BestURL(items[:3], ""); got != "" {
		t.Errorf("BestURL without web ports = %q", got)
	}
}

func TestBestURLPrefersClosest(t *testing.T) {
	web := func(name, path string, port int) Item {
		u := "http://localhost:" + strconv.Itoa(port)
		return Item{Name: name, Kind: "process", Path: path, URL: u, Ports: []Port{{port, u, "web"}}}
	}
	items := []Item{
		web("storybook", "/r/packages/ui", 6006),
		web("playground", "/r/playground", 5173),
		web("deep", "/r/a/b/c", 3000),
	}
	for dir, want := range map[string]string{
		"/r/playground":     "http://localhost:5173", // exact folder
		"/r/playground/src": "http://localhost:5173", // inside it
		"/r/packages/ui":    "http://localhost:6006",
		"/r":                "http://localhost:5173", // repo root: shallowest subfolder
		"/elsewhere":        "http://localhost:5173", // no closeness: shallowest
	} {
		if got := BestURL(items, dir); got != want {
			t.Errorf("BestURL(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestMergeDDEV(t *testing.T) {
	got := mergeDDEV([]Item{
		{Name: "site", Kind: "ddev", URL: "https://site.ddev.site", Ports: []Port{{8026, "", "mail"}}},
		{Name: "ddev-site", Kind: "docker", ddevSite: "site", Ports: []Port{{28832, "mysql://localhost:28832", "db"}}},
		{Name: "ddev-gone", Kind: "docker", ddevSite: "gone", Ports: []Port{{1, "", "other"}}},
	})
	if len(got) != 2 || len(got[0].Ports) != 2 || got[0].Ports[1].Port != 28832 || got[1].Name != "ddev-gone" {
		t.Fatalf("got %+v", got)
	}
}
