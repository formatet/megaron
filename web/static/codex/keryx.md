**Keryx** is Megaron's command-line client — the herald. It talks to the same server as the browser and can do everything a Wanax can do, including a few things the browser cannot do yet ([[rough-edges]]).

You do not need it to play. It exists for players who prefer a terminal, for automation, and for AI players.

## Getting started

Keryx is a single program; ask whoever runs your world for a copy. Then:

```
export POLEIA_SERVER=https://megaron.formatet.se
keryx login --username <your name>
keryx join
keryx status
```

`keryx actions` is the command to reach for when you are stuck: it lists what you can do right now. `keryx --help`, and `--help` after any command, explain the rest.

## A few useful commands

```
keryx status                 # your realm at a glance
keryx notifications          # the archive — what happened while you were away
keryx dispatches             # which kinds of news are pushed to you live
keryx map                    # the map around you
keryx city                   # people and placement in a city
keryx goods                  # stock and production
keryx wants                  # who needs what, among cities you have contacted
keryx password               # change your password
keryx occupation order ...   # sack, burn or annex an occupied city
```

Orders given through Keryx obey the same rules as in the browser: they travel by [[runners|Runner]] and take days ([[time]]).

`POLEIA_CONFIG` selects a separate configuration file. `POLEIA_SERVER` and `POLEIA_TOKEN` override connection settings. Command failures show the error and a pointer to `keryx <command> --help`. March previews call the soldiers' silver cost **Pay**.

With exactly one active world, commands select and save it automatically. To choose explicitly, run `keryx worlds` and then `keryx worlds --select <world-id>`. City rows show both the placement number and `(q,r)`; use those coordinates with `build --hex q,r`.

Use a city name or province ID either positionally or with `--province` in `city`, `placements`, `status` and `build`. For placement: `keryx place Knossos fish 13 -1` or `keryx place --province Knossos fish 13 -1`. Put flags before the good when using a negative delta. If no citizens work that good on the chosen hex, the result lists where they do work it. `founding settle --yes` skips the question. Travel estimates use days; if an arrival cannot be calculated it is reported as unavailable.
