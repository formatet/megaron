# Varvet — fixture isolation and lock-order investigation

Contract: identify the exact fixture row causing the global startup transition to fail after economy/combat. Preserve intentional production semantics: the migration cutover runs once before workers and covers existing jobs in all worlds, without inventing missing deadlines. If only tests are at fault, clean their own rows rather than hiding other worlds in production. Investigate real recruitment and shipyard paths in two PostgreSQL sessions, record blocking/deadlock evidence; no production fix without reporting and STOPP. Own fresh DBs only. Full Go suite once after any authorized test-only changes. No merge/push/deploy.

This verifies test trust and concurrency around shipbuilding, a substrate for the geography → shortage → bronze → elite chain, not a player observation of that chain. No player-facing behavior or new visual surface is planned.
