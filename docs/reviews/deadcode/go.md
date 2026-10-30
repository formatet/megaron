# E5 Go dead-code cleanup

## Contract
Problem: unreachable functions obscure the active ownership and implementation paths.
Player truth: behavior and reachable API/CLI contracts remain unchanged.
Invariant: remove only functions reported dead both with and without tests, after checking interface/reflection, routes, tools and gated-code intent.
Scope: Go dead functions and imports/types made obsolete solely by their removal.
Non-scope: Kingdoms, gameplay rules, schema, client surfaces, production rewrites.
Acceptance: current-toolchain analysis in both modes; audited removal inventory; builds/vet/focused tests; parent full fresh-DB suite before merge.
Stop: uncertain dynamic caller or an intended gated implementation.
Proof: baseline reports, per-symbol references, negative-control mutant, final analysis/build/tests.
Gate: infrastructure supporting chain verification, no new player surface.
