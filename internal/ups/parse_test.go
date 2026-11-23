package ups

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseLsofListen(t *testing.T) {
	out := []byte("p80321\ncnode\nf23\nn127.0.0.1:6006\nf24\nn[::1]:6006\n" +
		"p13244\ncOrbStack\nf10\nn*:80\nf11\nn*:443\n")
	got := parseLsofListen(out)
	want := []listener{
		{pid: 80321, cmd: "node", ports: []int{6006}},
		{pid: 13244, cmd: "OrbStack", ports: []int{80, 443}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestParseLsofCwdAndPs(t *testing.T) {
	cwd := parseLsofCwd([]byte("p80321\nfcwd\nn/Users/a/code/x\np1\nfcwd\nn/\n"))
	if cwd[80321] != "/Users/a/code/x" || cwd[1] != "/" {
		t.Fatalf("cwd = %v", cwd)
	}
	ps := parsePs([]byte("74166 /Applications/LM Studio.app/Contents/MacOS/LM Studio\n   80321 node\n"))
	if ps[74166] != "/Applications/LM Studio.app/Contents/MacOS/LM Studio" || ps[80321] != "node" {
		t.Fatalf("ps = %v", ps)
	}
}

func TestUnder(t *testing.T) {
	for _, c := range []struct {
		path, dir string
		want      bool
	}{
		{"/Users/a/code", "/Users/a", true},
		{"/Users/a", "/Users/a", false},
		{"/Users/ab", "/Users/a", false},
		{"/", "/Users/a", false},
		{"/Users/a/x", "", false},
	} {
		if got := under(c.path, c.dir); got != c.want {
			t.Errorf("under(%q, %q) = %v", c.path, c.dir, got)
		}
	}
}

func TestDockerPorts(t *testing.T) {
	got := dockerPorts("127.0.0.1:54329->5432/tcp, 127.0.0.1:8025-8026->8025-8026/tcp, " +
		"0.0.0.0:8080->80/tcp, :::8080->80/tcp, 127.0.0.1:9001->9000/tcp, 9000/tcp")
	want := []Port{
		{54329, "postgres://localhost:54329", "db"},
		{8025, "http://localhost:8025", "mail"},
		{8026, "https://localhost:8026", "mail"},
		{8080, "http://localhost:8080", "web"},
		{9001, "localhost:9001", "other"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if u := bestPortURL(got); u != "http://localhost:8080" {
		t.Errorf("bestPortURL = %q", u)
	}
}

func TestParseDockerPS(t *testing.T) {
	out := []byte(`{"Names":"llmtodo-postgres-1","Ports":"127.0.0.1:54329->5432/tcp","Status":"Up 3 hours","Labels":"com.docker.compose.project=llmtodo,com.docker.compose.service=postgres,com.docker.compose.project.working_dir=/Users/a/code/llmtodo,desc=a, b"}
{"Names":"ddev-x-db","Ports":"127.0.0.1:28832->3306/tcp","Status":"Up","Labels":"com.ddev.platform=ddev,com.ddev.site-name=x"}
{"Names":"ddev-router","Ports":"127.0.0.1:80->80/tcp","Status":"Up","Labels":"com.ddev.platform=ddev"}
`)
	got := parseDockerPS(out)
	if len(got) != 3 {
		t.Fatalf("want 3 items, got %+v", got)
	}
	it := got[0]
	if it.Name != "llmtodo" || it.Runs[0] != "docker postgres" || it.Path != "/Users/a/code/llmtodo" ||
		it.URL != "postgres://localhost:54329" {
		t.Fatalf("got %+v", it)
	}
	if got[1].ddevSite != "x" || got[1].Internal {
		t.Errorf("ddev site container = %+v", got[1])
	}
	if !got[2].Internal {
		t.Errorf("ddev-router should be internal: %+v", got[2])
	}
	if l := parseLabels("desc=a, b,k=v"); l["desc"] != "a, b" || l["k"] != "v" {
		t.Fatalf("labels = %v", l)
	}
}

func TestParseDDEV(t *testing.T) {
	out := []byte(`{"level":"info","raw":[
		{"name":"site","status":"running","approot":"/Users/a/code/site","httpsurl":"https://site.ddev.site","primary_url":"https://site.ddev.site","mailpit_https_url":"https://site.ddev.site:8026","type":"wordpress"},
		{"name":"old","status":"stopped","approot":"/Users/a/code/old","httpsurl":"https://old.ddev.site"}]}`)
	got, err := parseDDEV(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "site" || got[0].URL != "https://site.ddev.site" ||
		!reflect.DeepEqual(got[0].Ports, []Port{{8026, "https://site.ddev.site:8026", "mail"}}) {
		t.Fatalf("got %+v", got)
	}
}

func TestGroupProcesses(t *testing.T) {
	home := "/Users/a"
	early, late := time.Unix(1000, 0), time.Unix(2000, 0)
	got := groupProcesses([]proc{
		{listener: listener{7933, "node", []int{9229, 62264, 5173}}, cwd: home + "/code/bg/playground", exe: "node",
			args: "node /Users/a/code/bg/node_modules/.pnpm/vite@8.3.8_esbuild@0.28.2/node_modules/vite/bin/vite.js", started: &late},
		{listener: listener{7935, "workerd", []int{62265}}, cwd: home + "/code/bg/playground",
			exe: home + "/code/bg/node_modules/.bin/workerd", args: home + "/code/bg/node_modules/.bin/workerd serve", started: &early},
		{listener: listener{82512, "2.1.330", []int{55112}}, cwd: home + "/code/x", exe: "claude", args: "claude"},
		{listener: listener{98830, "llmtodo", []int{7878}}, cwd: "/", exe: home + "/Applications/llmtodo.app/Contents/MacOS/llmtodo"},
		{listener: listener{845, "rapportd", []int{49312}}, cwd: "/", exe: "/usr/libexec/rapportd", args: "/usr/libexec/rapportd"},
		{listener: listener{13244, "OrbStack Helper", []int{80}}, cwd: "/", exe: "/Applications/OrbStack.app/Contents/MacOS/OrbStack Helper"},
	}, home)

	if len(got) != 5 {
		t.Fatalf("want 5 items (vite+workerd merged), got %d: %+v", len(got), got)
	}
	pg := got[0]
	if pg.Name != "playground" || pg.URL != "http://localhost:5173" || len(pg.Ports) != 4 ||
		!reflect.DeepEqual(pg.Runs, []string{"vite", "workerd"}) || !reflect.DeepEqual(pg.PIDs, []int{7933, 7935}) ||
		!strings.Contains(pg.Command, "vite.js") || pg.Started == nil || !pg.Started.Equal(early) {
		t.Errorf("playground = %+v", pg)
	}
	roles := []string{}
	for _, p := range pg.Ports {
		roles = append(roles, p.Role)
	}
	if !reflect.DeepEqual(roles, []string{"web", "inspector", "ephemeral", "ephemeral"}) {
		t.Errorf("roles = %v", roles)
	}
	if rt := got[4]; !rt.Internal || !rt.runtime {
		t.Errorf("OrbStack should be an internal runtime: %+v", rt)
	}
	if got[1].Kind != "claude" || got[1].Name != "x" {
		t.Errorf("claude = %+v", got[1])
	}
	if it := got[2]; it.Name != "llmtodo" || it.Path != home+"/Applications/llmtodo.app" || it.System {
		t.Errorf("app = %+v", it)
	}
	if !got[3].System {
		t.Errorf("rapportd should be System: %+v", got[3])
	}
}

func TestDetectTool(t *testing.T) {
	for args, want := range map[string]string{
		"node /x/node_modules/.pnpm/storybook@10.6.8_react@19/node_modules/storybook/dist/bin": "storybook",
		"node /x/node_modules/.bin/next dev":                                                   "next",
		"node /x/server.js":                                                                    "node",
		"node /x/nextcloud/server.js":                                                          "node",
	} {
		if got := detectTool(args, "node", "node"); got != want {
			t.Errorf("detectTool(%q) = %q, want %q", args, got, want)
		}
	}
}

func TestIsContainerRuntime(t *testing.T) {
	for cmd, want := range map[string]bool{
		"OrbStack Helper": true, "OrbStack": true, "com.docker.backend": true, "node": false,
	} {
		if got := isContainerRuntime(cmd); got != want {
			t.Errorf("isContainerRuntime(%q) = %v", cmd, got)
		}
	}
}
