```
keryx status                 # your realm at a glance
keryx codex bronze           # read a Codex article
keryx notifications          # the archive — what happened while you were away
keryx notifications --all    # every kind, each event shown separately (latest 100)
keryx dispatches             # which kinds of news are pushed to you live
keryx map                    # the map around you
keryx city                   # people and placement in a city
keryx goods                  # stock and production
keryx password               # change your password
keryx occupation order ...   # sack, burn or annex an occupied city
```

`notifications --all` includes the historical kinds normally hidden or grouped in the default feed. It shows the latest 100 notifications; combine it with `--unread` for unread rows only. It does not mark anything read. Use `--kind` or `--exclude` separately to select kinds.

Orders given through Keryx obey the same rules as in the browser: they travel by [[runners|Runner]] and take game days ([[time]]).

`POLEIA_CONFIG` selects a separate configuration file. `POLEIA_SERVER` and `POLEIA_TOKEN` override connection settings. Command failures show the error and a pointer to `keryx <command> --help`. March previews call the soldiers' silver cost **Pay**.
