# Storm on an open worked-water hex

Contract: a storm visible on the world map must also appear in its open hex panel when it arrives after the panel was opened. The same storm block belongs in the city's selected workplace hex. Preserve other panel controls and drafts while weather refreshes. Web display only; storm movement, memory and FOW rules do not change.

Baseline ce7282b4, fresh process/schema220: actual Firefox clicks on worked coastal water show the initial storm correctly at1280/390. A second scenario opens the worked hex while the real storm is elsewhere, moves its saved track back, and waits for the normal30s /storms poll. Both clients have the live storm in State and draw it, while the panel has zero storm blocks. See baseline/observations.json and screenshots. No DOM or API substitutions.

Plan: one shared coordinate-scoped storm slot; refresh only those slots when normal storm data loads/polls/naming responses arrive. Reuse in world hex and city workplace details. API remains authoritative. Keryx already has `storms` with exact coordinates and `status` fishing loss; no nonexistent `hex` command promised. Codex explains both views. Verify regression tests, native baseline→fixed twice1280/390, physical mutant, source provenance and deployment.
