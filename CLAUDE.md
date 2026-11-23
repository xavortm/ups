# ups

Go CLI listing running local projects (DDEV sites, Docker containers, TCP listeners).

- Go version pinned in `mise.toml`. Commands: `mise run test`, `mise run dev [-- flags]`, `mise run build`.
- `main.go`: flags + table/JSON output. `internal/ups/`: one collector per source
  (`ddev.go`, `docker.go`, `process.go`), merged in `ups.go` `Collect`.
- Collectors shell out to `lsof`, `ps`, `docker ps --format json`, `ddev list -j` (stdlib only).
  Parsing is split into pure `parse*` functions tested with fixtures in `parse_test.go`;
  add a fixture when a new output shape shows up.
- A collector whose CLI is missing/fails returns an error and is silently skipped.
- Dedup rules: OrbStack/Docker Desktop listeners are dropped (Docker collector covers them);
  DDEV containers are dropped (DDEV collector reports the site).
- Processes sharing a folder merge into one item (`groupProcesses`); `portRank` puts the
  browser port first; `detectTool` names the dev tool from `ps args`.
- Claude Code sessions are kind `claude` and shown as a footer line, not table rows.
- Processes outside $HOME (cwd and executable) are `System` and hidden unless `-a`.
- macOS only for now (`lsof` flags, `ps -o comm=`).
