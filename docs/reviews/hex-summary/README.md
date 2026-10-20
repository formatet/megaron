# Short founding-site hex view

Contract: proves geography. Timothy finds the ordinary hex view too verbose.
An empty hex during founding must show a short site assessment, with the full
forecast and individual hexes available only in one closed Details section.
Reuse the existing Host assessment (food, timber, stone, ships), keep warnings and
visible/unknown limits, preserve all orders and forecast API parameters.
Scope: render/map.js founding-site view; no mechanics, API, CLI or wiki change.
Acceptance: closed by default, details accessible, no lost forecast data, stale
responses cannot replace a different selected hex. BILD before integration.
Baseline: existing five Host panel tests GREEN; current empty-hex renderer inserts
the entire shared forecast directly into ip-found-preview.

Validation: all 680 JS tests GREEN. `python3 tools/hex_summary_check.py` drives a real canvas click into the real hex panel using an explicit read-only API fixture matching the reported food balance. Closed summary and expanded original forecast checked; screenshots `/tmp/megaron-hex-summary/{closed,details}.png`. This is a frontend fixture proof, not a fresh backend acceptance world. BILD review before integration remains.
