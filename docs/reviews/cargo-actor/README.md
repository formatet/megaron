# Sea cargo displays its ship

Contract: Timothy's stone cargo is naval and bound to his galley, but the map
unconditionally draws a caravan. Reuse the existing carrying-unit sprite for
naval cargo, caravan for land cargo. Preserve position, route, FOW and quantities.
Scope: transport actor selection and semantic render verification. FUNCTION:
object identity, not new art/layout/palette. No server or gameplay change.
Acceptance: own freighting galley draws the same sprite as that galley moving;
land freight keeps its caravan. Existing full683JS baseline from parent ETA.

684JS GREEN. Actual canvas paint operations for a bound galley equal that
same galley sailing and differ from the pack-animal caravan. Land cargo keeps
its old paint operations. Restoring the old universal-caravan selection makes
the named semantic render test RED, restored code GREEN. Go is unchanged.
