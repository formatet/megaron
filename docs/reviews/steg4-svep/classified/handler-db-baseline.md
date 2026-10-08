# DB-anrop per handlerfil — daf740dd

AST-call-sites i produktionsfiler: Exec, Query, QueryRow, Begin, BeginTx, SendBatch, CopyFrom på h.pool/pool/tx/db. Scan/Commit/Rollback/Close räknas inte; r.URL.Query är uttryckligen utesluten. Även hjälpfunktioner i handlerpaketet ingår. Måttet räknar kodställen, inte frågor per request eller kostnad. Alla 35 filer, även nollrader, ingår.

| Fil | Totalt | QueryRow | Query | Exec | Begin/BeginTx | SendBatch/CopyFrom |
|---|---:|---:|---:|---:|---:|---:|
| server/api/handlers/agora.go | 0 | 0 | 0 | 0 | 0 | 0 |
| server/api/handlers/alfatestinfo.go | 0 | 0 | 0 | 0 | 0 | 0 |
| server/api/handlers/auth.go | 0 | 0 | 0 | 0 | 0 | 0 |
| server/api/handlers/capabilities.go | 1 | 1 | 0 | 0 | 0 | 0 |
| server/api/handlers/create_metropolis.go | 7 | 2 | 0 | 5 | 0 | 0 |
| server/api/handlers/db.go | 7 | 5 | 2 | 0 | 0 | 0 |
| server/api/handlers/dispatch_preferences.go | 3 | 0 | 1 | 2 | 0 | 0 |
| server/api/handlers/foreign_units.go | 2 | 0 | 2 | 0 | 0 | 0 |
| server/api/handlers/found_metropolis.go | 16 | 8 | 2 | 5 | 1 | 0 |
| server/api/handlers/gift_destinations.go | 3 | 1 | 2 | 0 | 0 | 0 |
| server/api/handlers/god.go | 2 | 0 | 2 | 0 | 0 | 0 |
| server/api/handlers/goods.go | 1 | 0 | 1 | 0 | 0 | 0 |
| server/api/handlers/helpers.go | 2 | 1 | 0 | 1 | 0 | 0 |
| server/api/handlers/join.go | 10 | 6 | 0 | 3 | 1 | 0 |
| server/api/handlers/kingdom.go | 57 | 31 | 4 | 17 | 5 | 0 |
| server/api/handlers/logistics.go | 4 | 0 | 0 | 3 | 1 | 0 |
| server/api/handlers/messenger.go | 44 | 24 | 3 | 12 | 5 | 0 |
| server/api/handlers/messenger_passage.go | 10 | 9 | 1 | 0 | 0 | 0 |
| server/api/handlers/nomadic_host.go | 3 | 2 | 0 | 1 | 0 | 0 |
| server/api/handlers/notifications.go | 4 | 0 | 1 | 3 | 0 | 0 |
| server/api/handlers/province.go | 114 | 70 | 27 | 11 | 6 | 0 |
| server/api/handlers/reports.go | 3 | 2 | 1 | 0 | 0 | 0 |
| server/api/handlers/retreat_default.go | 0 | 0 | 0 | 0 | 0 | 0 |
| server/api/handlers/rural.go | 1 | 0 | 1 | 0 | 0 | 0 |
| server/api/handlers/settlement.go | 42 | 18 | 8 | 12 | 4 | 0 |
| server/api/handlers/settlement_placement.go | 22 | 6 | 6 | 7 | 3 | 0 |
| server/api/handlers/standing_order.go | 14 | 5 | 3 | 4 | 2 | 0 |
| server/api/handlers/trade_journey.go | 2 | 0 | 2 | 0 | 0 | 0 |
| server/api/handlers/unit.go | 39 | 19 | 10 | 6 | 4 | 0 |
| server/api/handlers/unit_march_preview.go | 1 | 1 | 0 | 0 | 0 | 0 |
| server/api/handlers/unit_repair.go | 6 | 4 | 0 | 1 | 1 | 0 |
| server/api/handlers/web.go | 11 | 10 | 1 | 0 | 0 | 0 |
| server/api/handlers/world.go | 24 | 4 | 14 | 3 | 1 | 2 |
| server/api/handlers/world_guard.go | 2 | 2 | 0 | 0 | 0 | 0 |
| server/api/handlers/ws.go | 0 | 0 | 0 | 0 | 0 | 0 |
