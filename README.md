# ups

What's up and running on this machine? `ups` lists local projects in one table:

- **DDEV** sites (`ddev list`), with their `*.ddev.site` URLs and project folders
- **Docker / OrbStack** containers with published ports and their compose folder
- **Processes** listening on TCP ports (`lsof`), with the folder they were started from

```sh
mise run dev            # table
mise run dev -- --json  # JSON
mise run dev -- -a      # include system/app listeners (AirPlay, Raycast, …)
mise run build          # -> bin/ups
```

Missing tools (no Docker, no DDEV) are skipped, not errors.

Planned: `ups -i`, a Bubble Tea TUI (filter, open URL, kill, copy path) in Catppuccin Mocha.
