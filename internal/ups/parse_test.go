package ups

import (
	"reflect"
	"testing"
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

func TestDockerURLs(t *testing.T) {
	got := dockerURLs("127.0.0.1:54329->5432/tcp, 127.0.0.1:8025-8026->8025-8026/tcp, " +
		"0.0.0.0:8080->80/tcp, :::8080->80/tcp, 9000/tcp")
	want := []string{
		"postgres://localhost:54329", "localhost:8025", "localhost:8026", "http://localhost:8080",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestParseDockerPS(t *testing.T) {
	out := []byte(`{"Names":"llmtodo-postgres-1","Ports":"127.0.0.1:54329->5432/tcp","Status":"Up 3 hours","Labels":"com.docker.compose.project=llmtodo,com.docker.compose.service=postgres,com.docker.compose.project.working_dir=/Users/a/code/llmtodo,desc=a, b"}
{"Names":"ddev-x-web","Ports":"","Status":"Up","Labels":"com.ddev.platform=ddev"}
`)
	got := parseDockerPS(out)
	if len(got) != 1 {
		t.Fatalf("want 1 item (ddev skipped), got %+v", got)
	}
	it := got[0]
	if it.Name != "llmtodo" || it.Runs[0] != "docker postgres" || it.Path != "/Users/a/code/llmtodo" ||
		!reflect.DeepEqual(it.URLs, []string{"postgres://localhost:54329"}) {
		t.Fatalf("got %+v", it)
	}
	if l := parseLabels("desc=a, b,k=v"); l["desc"] != "a, b" || l["k"] != "v" {
		t.Fatalf("labels = %v", l)
	}
}

func TestParseDDEV(t *testing.T) {
	out := []byte(`{"level":"info","raw":[
		{"name":"site","status":"running","approot":"/Users/a/code/site","httpsurl":"https://site.ddev.site","type":"wordpress"},
		{"name":"old","status":"stopped","approot":"/Users/a/code/old","httpsurl":"https://old.ddev.site"}]}`)
	got, err := parseDDEV(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "site" || got[0].URLs[0] != "https://site.ddev.site" {
		t.Fatalf("got %+v", got)
	}
}

func TestGroupProcesses(t *testing.T) {
	home := "/Users/a"
	got := groupProcesses([]proc{
		{listener{7933, "node", []int{9229, 62264, 5173}}, home + "/code/bg/playground", "node",
			"node /Users/a/code/bg/node_modules/.pnpm/vite@8.3.8_esbuild@0.28.2/node_modules/vite/bin/vite.js"},
		{listener{7935, "workerd", []int{62265}}, home + "/code/bg/playground",
			home + "/code/bg/node_modules/.bin/workerd", home + "/code/bg/node_modules/.bin/workerd serve"},
		{listener{82512, "2.1.330", []int{55112}}, home + "/code/x", "claude", "claude"},
		{listener{98830, "llmtodo", []int{7878}}, "/", home + "/Applications/llmtodo.app/Contents/MacOS/llmtodo", ""},
		{listener{845, "rapportd", []int{49312}}, "/", "/usr/libexec/rapportd", "/usr/libexec/rapportd"},
	}, home)

	if len(got) != 4 {
		t.Fatalf("want 4 items (vite+workerd merged), got %d: %+v", len(got), got)
	}
	pg := got[0]
	if pg.Name != "playground" || pg.URLs[0] != "http://localhost:5173" || len(pg.URLs) != 4 ||
		!reflect.DeepEqual(pg.Runs, []string{"vite", "workerd"}) || !reflect.DeepEqual(pg.PIDs, []int{7933, 7935}) {
		t.Errorf("playground = %+v", pg)
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
