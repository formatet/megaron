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

Orders given through Keryx obey the same rules as in the browser: they travel by [[runners|Runner]] and take game-days ([[time]]).
