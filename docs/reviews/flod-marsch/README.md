# River march menu — contract

Problem: river targets offer land soldiers and hide galleys, contrary to server passability.
Player truth: a river offers ships; a ford remains land in the menu and permits either category.
Invariant: river_ford remains distinct from naval-only water; fog, landing and marching-ship rules stay valid.
Scope: existing renderer water classifier, destination creation, menu filtering, per-unit ford mission/preview, JS tests.
Acceptance: river => naval only; ford => land+naval; same legal naval exploration order in preview and send;
physical river removal gives a named red test; whole JS and required fresh Go/vet.
Non-scope: server, pathfinding, balance, text WIP.
Gate: blocks the geography → scarcity → bronze → elite chain by hiding a legal ship journey.


## Result

Core `8bed8d5a`, based on `b2e13145`; Claude reports merge/deploy `f5261fb4`.
Both map destination builders reuse the renderer's existing `isWaterTerrain` (river included,
ford excluded). River panel names stay river names, not “Sea”. Naval-only targets offer ships;
ford targets still look like land and permit both categories, as server `isPassable` specifies.
A naval ford order uses explore, with no land stance or colonization fields. Per-unit options
are shared by preview and send; shared target/duration are retained. Fog, loaded-ship landing,
fortification and marching-ship exclusion tests remain green. No server source changed.

Proof: **636 JS / zero skips**, fresh full Go on migration181 green (world151.194s), vet clean.
`mutations.py` physically removes river classification, ford eligibility, naval mission conversion,
and per-unit preview options; each produces a named failure, then restores identical source bytes
and gets green. `mutations.log` records SHA256s. The send regression observes actual `fetchAuth`
POST bodies; its deliberate API refusal avoids unrelated world refresh/audio state changes.

Two independent native processes use normal registration/join/coastal founding and real map
right-clicks (no SQL fixture writes, no HTTP overrides, no credential resets).
`native/` records the first successful UI/send; `native-final/` repeats on a fresh world and also
observes the physical ship entering **marching** after its command is delivered. Both health
responses carry core8bed8d5a/migration181, Chromium1280×900, zero page errors.
Preview honestly reports `unknown_terrain`, without an arrival estimate; dispatch is accepted
HTTP202 and the final run observes actual movement. Screenshot `native/river-menu.png` inspected
at native resolution. `live.py` reproduces the independent rig on random ports/own containers;
it leaves dev and the shared acceptance rig untouched. The first exploratory run assumed a
preview ETA was required, then correctly rejected that assumption; `live.log` retains it.

No layout/palette changed: this is a function/semantic check, with existing UI imagery as evidence.
No new BILD approval requirement is inferred. Claude reviewed and deployed the production core;
this report is the remaining docs handover. Text WIP remains on its separate branch.
