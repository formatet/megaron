# Pair table (before)

Open A, then open B. overlap = visible boxes intersect. **PROBLEM** = overlap and A still open (A covered, not closed). Reappears = A open again after B closed, having been covered.

## 1280 px

| A (first) | B (then) | overlap | A open with B | A open after B closed | verdict |
|---|---|---|---|---|---|
| hex | march | no | yes | yes | ok: apart, both stay |
| hex | drawer-city | yes | no | no | ok: closed |
| hex | drawer-war | yes | no | no | ok: closed |
| hex | drawer-report | yes | no | no | ok: closed |
| hex | codex | no | yes | yes | ok: apart, both stay |
| hex | dispatch | no | yes | yes | ok: apart, both stay |
| hex | search | no | yes | yes | ok: apart, both stay |
| hex | account | no | yes | yes | ok: apart, both stay |
| march | hex | no | yes | yes | ok: apart, both stay |
| march | drawer-city | yes | yes | yes | PROBLEM (covered, still open) |
| march | drawer-war | yes | yes | yes | PROBLEM (covered, still open) |
| march | drawer-report | yes | yes | yes | PROBLEM (covered, still open) |
| march | codex | no | yes | yes | ok: apart, both stay |
| march | dispatch | yes | yes | yes | PROBLEM (covered, still open) |
| march | search | no | yes | yes | ok: apart, both stay |
| march | account | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-city | hex | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-city | march | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-city | drawer-war | yes | no | no | ok: closed |
| drawer-city | drawer-report | yes | no | no | ok: closed |
| drawer-city | codex | no | yes | yes | ok: apart, both stay |
| drawer-city | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-city | search | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-city | account | no | yes | yes | ok: apart, both stay |
| drawer-war | hex | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-war | march | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-war | drawer-city | yes | no | no | ok: closed |
| drawer-war | drawer-report | yes | no | no | ok: closed |
| drawer-war | codex | no | yes | yes | ok: apart, both stay |
| drawer-war | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-war | search | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-war | account | no | yes | yes | ok: apart, both stay |
| drawer-report | hex | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-report | march | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-report | drawer-city | yes | no | no | ok: closed |
| drawer-report | drawer-war | yes | no | no | ok: closed |
| drawer-report | codex | no | yes | yes | ok: apart, both stay |
| drawer-report | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-report | search | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-report | account | no | yes | yes | ok: apart, both stay |
| codex | hex | no | yes | yes | ok: apart, both stay |
| codex | march | no | yes | yes | ok: apart, both stay |
| codex | drawer-city | no | yes | yes | ok: apart, both stay |
| codex | drawer-war | no | yes | yes | ok: apart, both stay |
| codex | drawer-report | no | yes | yes | ok: apart, both stay |
| codex | dispatch | yes | yes | yes | PROBLEM (covered, still open) |
| codex | search | yes | yes | yes | PROBLEM (covered, still open) |
| codex | account | yes | yes | yes | PROBLEM (covered, still open) |
| dispatch | hex | - | - | - | B did not open |
| dispatch | march | - | - | - | B did not open |
| dispatch | drawer-city | no | yes | yes | ok: apart, both stay |
| dispatch | drawer-war | no | yes | yes | ok: apart, both stay |
| dispatch | drawer-report | no | yes | yes | ok: apart, both stay |
| dispatch | codex | yes | yes | yes | PROBLEM (covered, still open) |
| dispatch | search | yes | yes | yes | PROBLEM (covered, still open) |
| dispatch | account | yes | yes | yes | PROBLEM (covered, still open) |
| search | hex | - | - | - | B did not open |
| search | march | - | - | - | B did not open |
| search | drawer-city | yes | yes | yes | PROBLEM (covered, still open) |
| search | drawer-war | yes | yes | yes | PROBLEM (covered, still open) |
| search | drawer-report | yes | yes | yes | PROBLEM (covered, still open) |
| search | codex | yes | yes | yes | PROBLEM (covered, still open) |
| search | dispatch | yes | yes | yes | PROBLEM (covered, still open) |
| search | account | yes | yes | yes | PROBLEM (covered, still open) |
| account | hex | - | - | - | B did not open |
| account | march | - | - | - | B did not open |
| account | drawer-city | no | yes | yes | ok: apart, both stay |
| account | drawer-war | no | yes | yes | ok: apart, both stay |
| account | drawer-report | no | yes | yes | ok: apart, both stay |
| account | codex | yes | yes | yes | PROBLEM (covered, still open) |
| account | dispatch | yes | yes | yes | PROBLEM (covered, still open) |
| account | search | yes | yes | yes | PROBLEM (covered, still open) |

## 1920 px

| A (first) | B (then) | overlap | A open with B | A open after B closed | verdict |
|---|---|---|---|---|---|
| hex | march | no | yes | yes | ok: apart, both stay |
| hex | drawer-city | yes | no | no | ok: closed |
| hex | drawer-war | yes | no | no | ok: closed |
| hex | drawer-report | yes | no | no | ok: closed |
| hex | codex | no | yes | yes | ok: apart, both stay |
| hex | dispatch | no | yes | yes | ok: apart, both stay |
| hex | search | no | yes | yes | ok: apart, both stay |
| hex | account | no | yes | yes | ok: apart, both stay |
| march | hex | no | yes | yes | ok: apart, both stay |
| march | drawer-city | no | yes | yes | ok: apart, both stay |
| march | drawer-war | no | yes | yes | ok: apart, both stay |
| march | drawer-report | no | yes | yes | ok: apart, both stay |
| march | codex | no | yes | yes | ok: apart, both stay |
| march | dispatch | no | yes | yes | ok: apart, both stay |
| march | search | no | yes | yes | ok: apart, both stay |
| march | account | no | yes | yes | ok: apart, both stay |
| drawer-city | hex | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-city | march | no | yes | yes | ok: apart, both stay |
| drawer-city | drawer-war | yes | no | no | ok: closed |
| drawer-city | drawer-report | yes | no | no | ok: closed |
| drawer-city | codex | no | yes | yes | ok: apart, both stay |
| drawer-city | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-city | search | no | yes | yes | ok: apart, both stay |
| drawer-city | account | no | yes | yes | ok: apart, both stay |
| drawer-war | hex | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-war | march | no | yes | yes | ok: apart, both stay |
| drawer-war | drawer-city | yes | no | no | ok: closed |
| drawer-war | drawer-report | yes | no | no | ok: closed |
| drawer-war | codex | no | yes | yes | ok: apart, both stay |
| drawer-war | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-war | search | no | yes | yes | ok: apart, both stay |
| drawer-war | account | no | yes | yes | ok: apart, both stay |
| drawer-report | hex | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-report | march | no | yes | yes | ok: apart, both stay |
| drawer-report | drawer-city | yes | no | no | ok: closed |
| drawer-report | drawer-war | yes | no | no | ok: closed |
| drawer-report | codex | no | yes | yes | ok: apart, both stay |
| drawer-report | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-report | search | no | yes | yes | ok: apart, both stay |
| drawer-report | account | no | yes | yes | ok: apart, both stay |
| codex | hex | no | yes | yes | ok: apart, both stay |
| codex | march | no | yes | yes | ok: apart, both stay |
| codex | drawer-city | no | yes | yes | ok: apart, both stay |
| codex | drawer-war | no | yes | yes | ok: apart, both stay |
| codex | drawer-report | no | yes | yes | ok: apart, both stay |
| codex | dispatch | no | yes | yes | ok: apart, both stay |
| codex | search | no | yes | yes | ok: apart, both stay |
| codex | account | no | yes | yes | ok: apart, both stay |
| dispatch | hex | - | - | - | B did not open |
| dispatch | march | - | - | - | B did not open |
| dispatch | drawer-city | no | yes | yes | ok: apart, both stay |
| dispatch | drawer-war | no | yes | yes | ok: apart, both stay |
| dispatch | drawer-report | no | yes | yes | ok: apart, both stay |
| dispatch | codex | no | yes | yes | ok: apart, both stay |
| dispatch | search | yes | yes | yes | PROBLEM (covered, still open) |
| dispatch | account | yes | yes | yes | PROBLEM (covered, still open) |
| search | hex | - | - | - | B did not open |
| search | march | - | - | - | B did not open |
| search | drawer-city | no | yes | yes | ok: apart, both stay |
| search | drawer-war | no | yes | yes | ok: apart, both stay |
| search | drawer-report | no | yes | yes | ok: apart, both stay |
| search | codex | no | yes | yes | ok: apart, both stay |
| search | dispatch | yes | yes | yes | PROBLEM (covered, still open) |
| search | account | yes | yes | yes | PROBLEM (covered, still open) |
| account | hex | - | - | - | B did not open |
| account | march | - | - | - | B did not open |
| account | drawer-city | no | yes | yes | ok: apart, both stay |
| account | drawer-war | no | yes | yes | ok: apart, both stay |
| account | drawer-report | no | yes | yes | ok: apart, both stay |
| account | codex | no | yes | yes | ok: apart, both stay |
| account | dispatch | yes | yes | yes | PROBLEM (covered, still open) |
| account | search | yes | yes | yes | PROBLEM (covered, still open) |
