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

`keryx actions` is the command to reach for when you are stuck: it lists what you can do right now. `keryx --help`, and `--help` after any command, explain the rest. The rules themselves are in this Codex: `keryx codex` lists the articles and `keryx codex <id>` prints one.

## A few useful commands

```
keryx status                 # your realm at a glance
keryx codex bronze           # read a Codex article
keryx notifications          # the archive — what happened while you were away
keryx dispatches             # which kinds of news are pushed to you live
keryx map                    # the map around you
keryx city                   # people and placement in a city
keryx goods                  # stock and production
keryx password               # change your password
keryx occupation order ...   # sack, burn or annex an occupied city
```

Orders given through Keryx obey the same rules as in the browser: they travel by [[runners|Runner]] and take days ([[time]]).

`POLEIA_CONFIG` selects a separate configuration file. `POLEIA_SERVER` and `POLEIA_TOKEN` override connection settings. Command failures show the error and a pointer to `keryx <command> --help`. March previews call the soldiers' silver cost **Pay**.

With exactly one active world, commands select and save it automatically. To choose explicitly, run `keryx worlds` and then `keryx worlds --select <world-id>`. City rows show both the placement number and `(q,r)`; use those coordinates with `build --hex q,r`.

Use a city name or province ID either positionally or with `--province` in `city`, `placements`, `status` and `build`. For placement: `keryx place Knossos fish 13 -1` or `keryx place --province Knossos fish 13 -1`. Put flags before the good when using a negative delta. If no citizens work that good on the chosen hex, the result lists where they do work it. `founding settle --yes` skips the question. Travel estimates use days; if an arrival cannot be calculated it is reported as unavailable.

Hex selection accepts the ordinal shown by City, with or without `#`: `build --type lumbermill --hex 6` or `--hex '#6'`. Placement can use `place fish 6 +1`, `place fish '#6' +1`, or `place --hex 6 fish +1`; `--hex q,r` also works. Put flags before the good when giving a negative delta. `staff` is for city workplaces; hex-bound buildings are worked with `place` on their hex.


## Argument reminders

Start construction with `keryx build --type harbour`. A positional argument in `build`
is the **city**, as in `keryx build Knossos --type harbour`; it is not the building type.
Use `build --list` for the catalogue or `build --queue` for queued construction.

`keryx staff olive_press` shows that named workplace's staffing. Add `+1` or `-1`
to change it; the building name is required. Use `keryx city` or `keryx roster`
to find workplaces and assignments. Hex buildings use `place`.

Send a report with `keryx report --text "what happened"`; use `--kind confused`
for a question the rules did not answer. For colony founding in scripts, add
`--yes`: `keryx unit march --unit <id> --target 5,-3 --intent colonize --name Thapsos --yes`.
Without confirmation, the non-interactive text command only shows the forecast;
JSON callers receive a refusal until they add `--yes`.

To learn what a contacted city needs or can spare, ask its Wanax with `keryx messenger` and read the reply with `keryx inbox`. See [[messengers]] and [[trade]].

Temple workers use the same citizen pool as other jobs. Use `keryx staff temple +1` to assign one, `staff temple -1` to free one, and `roster` to see their assignments. See [[temples]].
