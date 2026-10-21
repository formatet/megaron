# Double storm drift

Contract: Timothy2026-10-20 requests twice the storm movement speed. Preserve
three connected distinct sea hexes, existing damage/density/FOW, and stored past
tracks. Scope: two existing legal shape steps per future tick, weather tests and
Codex explanation. No visual layout, ship routing, new event/schema, or retroactive
weather changes. Proves the geography gate. Acceptance: deterministic straight
sea drift doubles, all shapes stay connected/on sea, duplicate scans do nothing,
late scan equals daily scans. Baseline: targeted fresh193 storm tests before edits.

Targeted fresh193 storm suite GREEN. Physical control StormStepsPerTick=1
fails TestSeaStormWeather_DriftExceedsOldSpeedBound: twelve ticks advance
3.833 hexes, within the former maximum4. Restored speed2 passes the bound,
connected/sea-only properties and duplicate/late-scan controls.
