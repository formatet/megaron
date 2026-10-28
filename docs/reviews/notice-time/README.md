# Codex notice timing

Contract: close todo's missing rumour age and civilian hunger recovery threshold. Text and constant-coupled tests only; no rule changes. Rumours cease propagation 72 real hours after receipt; civilian hunger warnings clear after five consecutive observed, fully fed game days. Supports understanding news and food in the player progression chain.

Both tests failed against old text, passed with corrected text, failed independently with constants changed (3→4 calendar days, 5→6 ticks), then passed after byte-exact restoration. Codex and day guards: 29/29. Full fresh Go suite passed in preceding test-only harbour slice; this slice's new DB-free tests pass. Player comprehension remains for alpha testing.
