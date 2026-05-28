# THALASSA — Claude Workspace Context

Keep this file updated as decisions are locked. If code and this file conflict, trust
the code — then fix this file.

**Before starting any task:** read the relevant vault documents below.
**Before ending any session:** update `thalassa_todo.md` to reflect what changed.
**If a design decision is made or revised:** update the relevant vault document
immediately — do not defer it.

---

## Vault documents (`~/Dokument/myltavault/`)

| Document | Contains |
|---|---|
| [`thalassa_todo.md`](~/Dokument/myltavault/thalassa_todo.md) | Sprint backlog, implementation status, known bugs |
| [`thalassa_designprinciper.md`](~/Dokument/myltavault/thalassa_designprinciper.md) | Philosophy, why each major decision was made |
| [`thalassa_settlement.md`](~/Dokument/myltavault/thalassa_settlement.md) | Province/settlement split, loyalty 1–4, revolt, messengers, info channels |
| [`thalassa_kharis.md`](~/Dokument/myltavault/thalassa_kharis.md) | Full kharis/religion system — belongs on settlements table |
| [`thalassa_kingdoms.md`](~/Dokument/myltavault/thalassa_kingdoms.md) | Kingdom structure, elections, borrowed armies, trade monopoly |
| [`thalassa_ekonomi_och_handel.md`](~/Dokument/myltavault/thalassa_ekonomi_och_handel.md) | **Current sprint spec** — goods catalog, production rules, bronze chain, price formula |
| [`thalassa_handel.md`](~/Dokument/myltavault/thalassa_handel.md) | v1 trade design: ships, caravans, ports, gossip, mobile play |
| [`thalassa_worldbuilding.md`](~/Dokument/myltavault/thalassa_worldbuilding.md) | Setting, cultures, Eras, Sea Peoples, Wanax identity |
| [`thalassa_backend.md`](~/Dokument/myltavault/thalassa_backend.md) | Architecture, FOSS/self-hosting philosophy, network design |
| [`thalassa_notifikationer.md`](~/Dokument/myltavault/thalassa_notifikationer.md) | Push notification taxonomy, BeReal mobile model |
| [`thalassa_titlar_och_militär.md`](~/Dokument/myltavault/thalassa_titlar_och_militär.md) | Title schema, Lochagos/Strategos/Nauarchos hierarchy (v1) |
| [`thalassa_prayers.md`](~/Dokument/myltavault/thalassa_prayers.md) | Prayer table, rites, divine actions (v1) |

---

## What this project is

Persistent asynchronous multiplayer grand strategy, mythic Bronze Age eastern
Mediterranean. 100 players per world. You are a **Wanax** ruling a network of
settlements. Kingdoms form organically (3–12 players). The world runs whether
you are online or not.

Tone: serious, warm, human-scale. Moomin Valley in the Bronze Age Aegean.
Inspirations: Utopia (1998), Settlers 2, EU4, Crusader Kings, Merchant Prince 2,
Diplomacy (board game).

---

## Current implementation state

**Built and running:**
- Single settlement per player (limitation — multi-settlement is next)
- Resources: gold, food, lumber, stone, iron, kharis — lazy evaluation
- Buildings with build queue, army units, march/combat (deterministic)
- Kingdom formation (backend done, UI stub)
- JWT auth, WebSocket hub, Redis pub/sub, event sourcing

**Migrations applied:** 001 initial · 002 buildings fix · 003 mana→kharis · 004 wizard→priest

**Not yet built:** multi-settlement, colonies, trade, naval, kharis as event system,
era/collapse, Sea Peoples, tech tree, terrain-gated buildings.

Full backlog: see `thalassa_todo.md`.

---

## Locked design decisions

**Province vs. settlement:** these are separate database tables. A province is the
hex tile (terrain, resource rates). A settlement is the inhabited fortress (buildings,
garrison, loyalty, governor). An outpost = province row with no settlement row.
Full design: `thalassa_settlement.md`, `thalassa_expansion.md`.

**Loyalty scale:** 1–4 only. 1=disgruntled, 2=loyal (default), 3=devoted, 4=fervent
(war only). Never 0–100. Stored as event log, computed as projection. `thalassa_settlement.md`.

**Kharis:** not mana — a reciprocal relationship between settlement and god.
Belongs on the `settlements` table, not `provinces`. 5% floor always applies.
Gods punish neglect actively (horses die, ships sink). Full design: `thalassa_kharis.md`.

**Messengers:** physical units on the map, visible to all, contribute to fog-of-war.
Reply arrives only when messenger returns home. Can board trade ships/caravans.
Sacred — no player can intercept. Only gods can make one disappear. `thalassa_settlement.md`.

**Kingdom:** King + max 2 Advisors. No Spymaster, no General, no Ambassador.
Elections Sundays only, 7-day lock. Borrowed armies penalise king after day 7.
Full design: `thalassa_kingdoms.md`.

**Combat:** deterministic, no dice. Strength = Σ(units × value) + support.
Infantry ×1, Cavalry ×3, Priest ×2, Catapult = wall breacher only.
Wall modifier: L0=1.0×, L1=1.25×, L2=1.5×, L3=1.75×.

**Collapse/Eras:** hidden prestige algorithm, risk begins week 10, threshold hidden.
Cannot be stopped — only survived. Sea Peoples escalate. Islands sink. `thalassa_worldbuilding.md`.

---

## Technical decisions

**Stack:** Go 1.22+ · chi · PostgreSQL 16 (pgx/v5) · Redis 7 (go-redis) · gorilla/websocket · golang-migrate · log/slog · HTMX + vanilla JS

**Architecture:**
- Event sourcing: append-only `events` table. Never UPDATE game state directly.
- Lazy resource eval: store `(amount, rate_per_minute, calc_at)`, compute on read.
- Timed event queue in PostgreSQL, SKIP LOCKED, worker polls every 10s.
- WebSocket hub per world for real-time push.

**Column naming (do not deviate):**
- Resources: `gold_amount`, `gold_rate`, `gold_cap`, `gold_calc_at` — NOT `*_last_calc_at`
- Army: bare column names — `infantry`, `cavalry`, `catapult`, `priest`, `ship`

**Deployment:** `docker compose up` at project root. Migrations run on startup.
`.env.example` → `.env` before first run.

---

## Visual style (code-relevant)

Full spec: `thalassa_designprinciper.md` and `thalassa_worldbuilding.md`.

**Palette (needed for CSS/canvas):**

| Name | Hex | Use |
|---|---|---|
| AEGEAN | #2E86C1 | Sea |
| DEEP_SEA | #1B4F72 | Deep water |
| SHALLOW | #5DADE2 | Coastal shallows |
| FIRED_CLAY | #C0392B | Roof tiles, signal colour |
| TERRACOTTA | #E67E22 | Walls, paths, UI borders |
| SANDSTONE | #F0B27A | Building faces in sun |
| LINEN | #FAD7A0 | Light ground, beach |
| PLASTER | #F2EAD3 | Shaded walls, UI panels |
| ORACLE_GOLD | #F9E79F | Divine glows, temple highlights only |
| DOOM_VIOLET | #6C3483 | Collapse signs, Sea Peoples |
| CHARCOAL | #1C1C1C | Outlines, text |

**Culture accents:** Akhaier #CA8A04 · Khemetiu #0E7490 · Kna'an #86198F · Thrakes #1D4ED8 · Pelasger #92400E · Hatti #374151

**Rules:** 1px CHARCOAL outline on all solid objects. No anti-aliasing. No gradients.
No rounded corners. Background terrain desaturated, foreground objects saturated.

---

## Terminology

| Use | Not |
|---|---|
| Wanax | Player / Lord |
| Kharis | Mana / energy |
| Era | Season / Round |
| Province | Hex / Territory |
| Settlement | City / Base |
| Kingdom | Alliance / Guild |
| Rite | Spell / Ability |
| March | Attack (verb) |
| Sea Peoples | Boss / Enemy |
| Collapse | Season end |

---

## Project file layout

```
Thalassa/
├── CLAUDE.md
├── docker-compose.yml / Dockerfile / .env.example
├── preview.html                     — standalone map renderer
├── music/                           — ABC source + midi/
└── server/
    ├── cmd/server/main.go
    ├── api/handlers/                — auth, province, kingdom, web, ws, db, join
    ├── internal/
    │   ├── auth/                    — JWT + middleware
    │   ├── combat/                  — arrival, build, train, resolver
    │   ├── events/                  — scheduler, store, worker
    │   ├── notify/                  — WebSocket hub
    │   ├── province/                — model, building specs, training, hex math
    │   ├── religion/                — stub
    │   └── world/                   — map gen, model
    └── db/migrations/
```
