# Pair table (after)

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
| march | drawer-city | yes | no | no | ok: closed |
| march | drawer-war | yes | no | no | ok: closed |
| march | drawer-report | yes | no | no | ok: closed |
| march | codex | no | yes | yes | ok: apart, both stay |
| march | dispatch | yes | no | no | ok: closed |
| march | search | no | yes | yes | ok: apart, both stay |
| march | account | yes | no | no | ok: closed |
| drawer-city | hex | yes | no | no | ok: closed |
| drawer-city | march | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-city | drawer-war | yes | no | no | ok: closed |
| drawer-city | drawer-report | yes | no | no | ok: closed |
| drawer-city | codex | no | yes | yes | ok: apart, both stay |
| drawer-city | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-city | search | yes | no | no | ok: closed |
| drawer-city | account | no | yes | yes | ok: apart, both stay |
| drawer-war | hex | yes | no | no | ok: closed |
| drawer-war | march | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-war | drawer-city | yes | no | no | ok: closed |
| drawer-war | drawer-report | yes | no | no | ok: closed |
| drawer-war | codex | no | yes | yes | ok: apart, both stay |
| drawer-war | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-war | search | yes | no | no | ok: closed |
| drawer-war | account | no | yes | yes | ok: apart, both stay |
| drawer-report | hex | yes | no | no | ok: closed |
| drawer-report | march | yes | yes | yes | PROBLEM (covered, still open) |
| drawer-report | drawer-city | yes | no | no | ok: closed |
| drawer-report | drawer-war | yes | no | no | ok: closed |
| drawer-report | codex | no | yes | yes | ok: apart, both stay |
| drawer-report | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-report | search | yes | no | no | ok: closed |
| drawer-report | account | no | yes | yes | ok: apart, both stay |
| codex | hex | no | yes | yes | ok: apart, both stay |
| codex | march | no | yes | yes | ok: apart, both stay |
| codex | drawer-city | no | yes | yes | ok: apart, both stay |
| codex | drawer-war | no | yes | yes | ok: apart, both stay |
| codex | drawer-report | no | yes | yes | ok: apart, both stay |
| codex | dispatch | yes | no | no | ok: closed |
| codex | search | yes | no | no | ok: closed |
| codex | account | yes | no | no | ok: closed |
| dispatch | hex | - | - | - | B did not open |
| dispatch | march | - | - | - | B did not open |
| dispatch | drawer-city | no | yes | yes | ok: apart, both stay |
| dispatch | drawer-war | no | yes | yes | ok: apart, both stay |
| dispatch | drawer-report | no | yes | yes | ok: apart, both stay |
| dispatch | codex | yes | no | no | ok: closed |
| dispatch | search | yes | no | no | ok: closed |
| dispatch | account | yes | no | no | ok: closed |
| search | hex | - | - | - | B did not open |
| search | march | - | - | - | B did not open |
| search | drawer-city | yes | no | no | ok: closed |
| search | drawer-war | yes | no | no | ok: closed |
| search | drawer-report | yes | no | no | ok: closed |
| search | codex | yes | no | no | ok: closed |
| search | dispatch | yes | no | no | ok: closed |
| search | account | yes | no | no | ok: closed |
| account | hex | - | - | - | B did not open |
| account | march | - | - | - | B did not open |
| account | drawer-city | no | yes | yes | ok: apart, both stay |
| account | drawer-war | no | yes | yes | ok: apart, both stay |
| account | drawer-report | no | yes | yes | ok: apart, both stay |
| account | codex | yes | no | no | ok: closed |
| account | dispatch | yes | no | no | ok: closed |
| account | search | yes | no | no | ok: closed |

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
| drawer-city | hex | yes | no | no | ok: closed |
| drawer-city | march | no | yes | yes | ok: apart, both stay |
| drawer-city | drawer-war | yes | no | no | ok: closed |
| drawer-city | drawer-report | yes | no | no | ok: closed |
| drawer-city | codex | no | yes | yes | ok: apart, both stay |
| drawer-city | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-city | search | no | yes | yes | ok: apart, both stay |
| drawer-city | account | no | yes | yes | ok: apart, both stay |
| drawer-war | hex | yes | no | no | ok: closed |
| drawer-war | march | no | yes | yes | ok: apart, both stay |
| drawer-war | drawer-city | yes | no | no | ok: closed |
| drawer-war | drawer-report | yes | no | no | ok: closed |
| drawer-war | codex | no | yes | yes | ok: apart, both stay |
| drawer-war | dispatch | no | yes | yes | ok: apart, both stay |
| drawer-war | search | no | yes | yes | ok: apart, both stay |
| drawer-war | account | no | yes | yes | ok: apart, both stay |
| drawer-report | hex | yes | no | no | ok: closed |
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
| dispatch | search | yes | no | no | ok: closed |
| dispatch | account | yes | no | no | ok: closed |
| search | hex | - | - | - | B did not open |
| search | march | - | - | - | B did not open |
| search | drawer-city | no | yes | yes | ok: apart, both stay |
| search | drawer-war | no | yes | yes | ok: apart, both stay |
| search | drawer-report | no | yes | yes | ok: apart, both stay |
| search | codex | no | yes | yes | ok: apart, both stay |
| search | dispatch | yes | no | no | ok: closed |
| search | account | yes | no | no | ok: closed |
| account | hex | - | - | - | B did not open |
| account | march | - | - | - | B did not open |
| account | drawer-city | no | yes | yes | ok: apart, both stay |
| account | drawer-war | no | yes | yes | ok: apart, both stay |
| account | drawer-report | no | yes | yes | ok: apart, both stay |
| account | codex | no | yes | yes | ok: apart, both stay |
| account | dispatch | yes | no | no | ok: closed |
| account | search | yes | no | no | ok: closed |
