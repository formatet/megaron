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
