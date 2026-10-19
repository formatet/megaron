# Server reproduction — Haiku 6 (b)

Base: `07e2eac4`, migration 189. Each command below creates its own empty PostgreSQL 16 database through `tools/gotest.sh`; HTTP requests use the real handlers, chi routes and Bearer authentication. Unit rows are seeded to reproduce the reported states, rather than replaying combat or recruitment history. No live database was inspected or changed.

| State and HTTP order | Observed result | Stored result |
|---|---|---|
| Completed land cohort, `garrison`, size 64; POST march, `intent=explore` | 202 | `marching`, size 64 |
| Unfinished land cohort, `forming`, size 40; same order | 422: `unit cannot march: status is 'forming' (must be 'garrison' or 'positioned')` | Still `forming`, size 40 |
| Land cohort, `garrison`, size 64, settlement = origin; POST reinforce | 200, `reinforcing=true` | `reinforcing=true` |
| Land cohort, `garrison`, size 62, another city of the same owner; POST reinforce | 422: `reinforce only works in the cohort's home city — march it back to its origin settlement first` | `reinforcing=false` |

The reported refusal while actually garrisoned at origin was **not reproduced**. A garrison in another own city reproduces the refusal, but this does not establish where Haiku 6's original unit stood. No mechanics or error strings changed. The inaccurate handler comment requiring exactly 100 men was corrected; the reinforce fixture's hypothetical kingdom-loan explanation was replaced with the existing own-city case.

Verification:

- Unchanged baseline: `tools/gotest.sh ./api/handlers/ -run 'Test(Reinforce_|March_FOWRule|Recall_FOW)' -v` — PASS, seven tests.
- After adding explicit 64/40 coverage: `tools/gotest.sh ./api/handlers/ -run 'Test(Reinforce_|March_ExploreCompleted64AndForming40|March_FOWRule|Recall_FOW)' -v` — PASS, eight tests including both new subtests.
- Full handlers package: `tools/gotest.sh ./api/handlers/` — PASS (38.108 s), another fresh database at migration 189.
- `git diff --check` — PASS.

Coverage: `unit_march_cohort_status_test.go` checks HTTP responses and persisted status/size. `reinforce_endpoint_test.go` checks origin acceptance and reinforces the away-city refusal assertion with its actual error and unchanged flag. Existing full-strength, training and naval refusal cases remain intact.
