# Six terrain production articles

Contract: replace vague production guidance with useful base workplaces and per-worker output for plains, river valley, delta, hills, olive grove and cedar forest, completed in that order. No game rules change. Supports choosing land and allocating citizens in the progression chain.

The DB-backed generator/test calls the city's loadHexOptionsForHexes owner on a terrain-only, unbuilt hex. It reads fully migrated production_rules and applies the same hexYield. Fixtures run in rolled-back transactions. Deposits, buildings and refining are explicitly excluded from base values; players are directed to the actual city view.

Each article generated and checked on fresh migration-224 databases, with Codex/time guards 29/29 before its commit/deploy. Six deliberately corrupted table rows produced six named failures. Restored originals passed together on a fresh DB. Missing tables also fail. Regenerate with tools/gotest.sh ./internal/economy -run TestCodexTerrainProduction -update-terrain-article=plains, then python3 tools/codex_facts.py (substitute the article slug).

Remaining: other terrains, deposit-specific examples and building/refining output comparisons. No claim that the entire terrain catalogue is complete.
