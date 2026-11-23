# ups

What's up and running on this machine? One table of local projects:

- **DDEV** sites with their `*.ddev.site` URL (plus Mailpit and DB ports)
- **Docker / OrbStack** containers with published ports and their compose folder
- **Processes** listening on TCP ports, grouped by the folder they run from, with the dev tool
  detected (vite, storybook, next, …) and the port you'd open ranked first

```
$ ups
NAME             URL                                  RUNS             FOLDER                                   PID
astro-wp-test    https://astro-wp-test.ddev.site  +5  ddev wordpress   ~/code/fueled/astro-wp-test
llmtodo          postgres://localhost:54329           docker postgres  ~/code/personal/llmtodo
playground       http://localhost:5173            +5  vite, workerd    ~/code/personal/boardgame-cf/playground  7933 +1

Claude Code sessions: boardgame-cf, scriber
4 system/app listeners hidden (ups -a to show)
```

## Usage

```sh
ups               # everything running
ups .             # just this project (its git repo; worktrees count as separate projects)
ups url [path]    # print the best URL for a project, e.g. curl "$(ups url)/api/health"
ups port 5173     # who is listening on a port (folder, git branch, command, start time)
```

Flags, anywhere on the line: `-a/--all` (include system/app listeners), `--json`, `-q/--quiet`.
Exit status: `0` found, `1` nothing running / port free, `2` usage error, so `ups . -q || pnpm dev` works.

## For agents

`ups . --json` gives each item's `url`, `ports[{port,url,role}]` (roles: web, db, cache, mail,
inspector, ephemeral, other), `runs`, `pids`, full `command`, `started`, `path` and
`git{root,branch,worktree,main_root}`. Use it to reuse an already-running dev server instead of
starting a second one, and `ups port <n>` to explain an "address already in use" error.
JSON field names are stable; new fields may be added.

## Develop

```sh
mise run test     # go vet + go test
mise run dev      # go run . (pass flags after --)
mise run build    # -> bin/ups
mise run install  # -> ~/.local/bin/ups (on PATH)
```

macOS only for now. Missing tools (no Docker, no DDEV) are skipped, not errors.
