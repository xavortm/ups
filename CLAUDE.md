# ups

Go CLI listing running local projects (DDEV sites, Docker containers, TCP listeners).

- Go pinned in `mise.toml`. Commands: `mise run test`, `mise run dev [-- args]`, `mise run build`.
- `main.go`: arg parsing (flags allowed after positionals), subcommands (list / `<path>` / `url` / `port`),
  table + detail + JSON output, exit codes (0 found, 1 none, 2 usage).
- `internal/ups/`: one collector per source (`ddev.go`, `docker.go`, `process.go`) shelling out to
  `ddev list -j`, `docker ps --format json`, `lsof`, `ps` (stdlib only). `ups.go` `Collect` merges
  them; `git.go` reads `.git` from disk (no git subprocess); `query.go` has Scope/ByPort/BestURL.
- `Item` JSON is a public interface for agents: add fields, don't rename/remove.
- Parsing lives in pure functions tested with real-output fixtures (`*_test.go`); add one per new shape.
- Rules: DDEV site containers merge into the DDEV item (`mergeDDEV`); DDEV router/ssh-agent and
  OrbStack's proxy are `Internal` (only `ups port` shows them); processes sharing a folder merge;
  `portRank` orders ports; `Scope` compares git roots so worktrees stay separate projects.
- Claude Code sessions are kind `claude`, shown as a footer line, not table rows.
- macOS only (`lsof` flags, `ps -o comm=/lstart=`).
