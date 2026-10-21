# Cargo uses the best approach to a port

Contract: the stone shipment chose the long northern route because each city's
first adjacent water hex was fixed before pathfinding. Treat each coastal endpoint
as connected to its adjacent navigable water and choose the least-cost reachable
pair. Preserve actual water-only saved paths, terrain pricing, authorisation,
old sealed journeys and no passage through inland terrain. No new diplomatic
access or permanent port side. Proves physical trade geography. Scope: pure trade
journey planner, regression tests and Codex. Acceptance: short southern approach
wins; unreachable first dock doesn't hide a reachable alternative; ordinary land
and water-to-water cases unchanged. Baseline: existing journey tests before edits.

Targeted fresh193 province/transport/API trade+gift+arrival tests GREEN.
Synthetic two-sided port has a long northern and a shorter southern waterway;
minimum-cost planning selects south. Isolating the first dock still leaves a
valid southern route. Restoring first-dock selection produces named RED.
No diplomacy rule changes: callers still authorise endpoints before planning.

Full fresh193 Go suite, province/transport/handler vet and683JS GREEN.
`tools/port_routing_readonly.py` reads the actual stone shipment and world
terrain in a READ ONLY transaction, then runs the pure planner locally. The
new path goes south and is shorter; exact counts in live-readonly.json.
No live order, shipment, unit or tile is changed by this proof.
