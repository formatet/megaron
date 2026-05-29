# POLEIA — Claude Workspace Context

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
| [`poleia_index.md`](~/Dokument/myltavault/poleia_index.md) | **Start here** — categorised index of all vault documents |
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
| [`thalassa_arkitekturrefaktor.md`](~/Dokument/myltavault/thalassa_arkitekturrefaktor.md) | 11-fas backend refactor plan — Clock, economy, religion, ruleset, notify |

---

## What this project is

Persistent asynchronous multiplayer grand strategy, mythic Bronze Age eastern
Mediterranean. 100 players per world. You are a **Wanax** ruling a network of
settlements. Kingdoms form organically (3–12 players). The world runs whether
you are online or not.

Tone: serious, warm, human-scale. Moomin Valley in the Bronze Age Aegean.
Inspirations: Utopia (1998), Settlers 2, EU4, Crusader Kings, Merchant Prince 2,
Diplomacy (board game).

**Name note:** The project is called **Poleia**. The sea in the game world is called
**The Thalassa** — the great primordial sea connecting all civilisations. This is
permanent flavour: `terrain = "sea"` in the DB, "The Thalassa" in all UI/map labels.

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

**Kingdom:** Basileus + members. No Spymaster, no General, no Ambassador.
Elections Sundays only, 7-day lock. Borrowed armies penalise basileus after day 7.
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

---

## Architecture rules (hard — do not deviate)

### Time
- **Never call `time.Now()` directly.** All time goes through `clock.Clock.Now()`.
- Inject `clock.Clock` via constructor. Use `clock.TestClock` in tests.
- Only `internal/events` and `main.go` may hold a `*clock.WallClock`.

### Event handlers (Fas 2.2 — idempotency)
Every handler registered with `events.Worker` must be safe to run twice.
Accepted patterns:
1. `SELECT … FOR UPDATE` → do work → `UPDATE processed=true` — all in one transaction.
2. `INSERT … ON CONFLICT (event_id) DO NOTHING` for projection writes.
If a handler is not idempotent, mark it with a `// TODO: idempotent` comment and file an issue.

### Events store outcomes, not intentions (Fas 2.3)
Probabilistic rolls happen **once** in the handler; the **result** is what goes in the event payload.
A `DivinePunishment` event must say `{"type":"cavalry_loss","amount":3}`, not `{"roll_pending":true}`.
No event may say "check if X happened" — it must say "X happened" or not exist.

### Event versioning (Fas 2.4)
Event schemas are **frozen in semantics forever**. Never change how an existing event type is interpreted.
To evolve: create a new event type (`MessengerArrivedV2`). Old handlers keep reading old types.

### Package dependency order (G1 — strict, no exceptions)
```
clock, events  ← zero internal deps
  ↑
economy, religion  ← may use clock, events
  ↑
settlement, province  ← may use economy, religion, clock, events
  ↑
combat, kingdom  ← may use settlement, province, economy, religion, clock, events
  ↑
messenger, notify  ← may use all above
  ↑
api/handlers  ← may use all
```
A package may import **downward only**. Upward communication goes via event emission.
Consumer interfaces are defined in the **consuming** package, never in the implementing one.

### Handler timeouts (G2)
`events.Worker` wraps every handler in `context.WithTimeout` (default 5 s).
Handlers **must** pass `ctx` to every DB call (`QueryContext`, `ExecContext`).
Three consecutive failures → dead-lettered (logged as ERROR, `failed_at` set).

### Auth (G3)
Bearer token in `Authorization` header — not httpOnly cookie.
HTMX wires it via `htmx:configRequest` listener in `web/static/js/auth.js`.
iOS client will use Keychain → Bearer directly. No CSRF tokens needed.

**Deployment:** `docker compose up` at project root. Migrations run on startup.
`.env.example` → `.env` before first run.

---

## Visual style (code-relevant)

Full spec: `thalassa_designprinciper.md` and `thalassa_worldbuilding.md`.

**CSS variables — always use these, never hardcode hex in templates or CSS:**

All colours live as custom properties in `web/static/poleia.css` `:root`. Use them everywhere.

| Variable | Hex | Name | Use |
|---|---|---|---|
| `--accent` | #C0392B | FIRED_CLAY | Primary action, section headings |
| `--border` | #E67E22 | TERRACOTTA | Panel borders, dividers |
| `--border-in` | #6E2C00 | ASH_BROWN | Inner shadow, recessed edge |
| `--bg` | #1C1C1C | CHARCOAL | Page background |
| `--bg-panel` | #F2EAD3 | PLASTER | Panel face |
| `--bg-raised` | #FAD7A0 | LINEN | Raised / highlight face |
| `--warm-white` | #FDFEFE | WARM_WHITE | Input field background |
| `--sandstone` | #F0B27A | SANDSTONE | Button face |
| `--text` | #1C1C1C | CHARCOAL | Body text on light |
| `--text-light` | #F2EAD3 | PLASTER | Body text on dark |
| `--text-dim` | #A04000 | LOAM | Muted / secondary text |
| `--gold` | #F9E79F | ORACLE_GOLD | Divine glow, global h1/h2/h3 |
| `--safe` | #D4AC0D | DRY_GRASS | Positive values, support intent |
| `--danger` | #6C3483 | DOOM_VIOLET | Collapse, danger states |
| `--blood` | #922B21 | BLOOD_RED | Combat banners only |
| `--sea` | #2E86C1 | AEGEAN | Sea, join-button |
| `--deep-sea` | #1B4F72 | DEEP_SEA | Sea text, deep water |

**Culture accents (canvas only — not in CSS variables):**
Akhaier #CA8A04 · Khemetiu #0E7490 · Kna'an #86198F · Thrakes #1D4ED8 · Pelasger #92400E · Hatti #374151

**Rules:** 1px CHARCOAL outline on all solid objects. No anti-aliasing. No gradients.
No rounded corners. Background terrain desaturated, foreground objects saturated.
Canvas renderer is exempt from CSS variables — it has its own internal palette.
Do not add inline `style="color:#..."` in templates — add a class to poleia.css instead.

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
| The Thalassa | The Sea (map label) |

---

## Project file layout

```
Poleia/
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
