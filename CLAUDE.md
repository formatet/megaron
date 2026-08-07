# MEGARON — Claude Workspace Context

Config file, not a knowledge base. **Instructions + pointers only — facts live in the vault and the code.**
If code and this file conflict, trust the code, then fix this file.
**Never quote a tunable number here as if it were an invariant** — thresholds, %, ranges and enums are
read from code or vault on demand. A `≥3` beside the word "invariant" makes an agent refuse to tune it.

- **Before a task:** read the relevant vault doc(s) — index at `~/Dokument/myltavault/megaron_moc.md` (**start here**).
- **Two gates** — mark every piece of work *blocks* / *proves* / *waits for* them:
  **(1) The chain:** a competent player completes geografi → brist → brons → elit via web and keryx, with no
  developer intervention and **without hitting a surface that doesn't exist**. Progress is measured in **game
  days, never wall time** ("in one session" was struck 2026-08-02 — at 60 min/tick the chain spans real days).
  **(2) Asynchronicity:** a Wanax away for nine hours must, on login, learn what happened, see what is heading
  their way and when it lands — **and have time to answer**. Sharp form: order travel + defender travel <
  attacker's remaining travel. Orders are physical; command is never instant.
- **Before ending a session:** update `megaron_todo.md` — four queues with caps (NU ≤5 · BESLUT ≤7 ·
  VERIFIERING · SENARE). Not a diary. Group new observations by likely shared root before writing them down.
- **When a design decision changes:** update the relevant vault doc immediately — don't defer.
- **Timestamps:** pull the actual wall clock from plain `date`, never a guessed or remembered time.
  **Never a `TZ=` prefix** — git-for-Windows ships no zoneinfo DB, so it silently falls back to UTC.
  Format `(YYYY-MM-DD HH:MM)`.
- Vault rights: read/write `~/Dokument/myltavault` freely without asking.
- **Loose design dumps** are an inbox, not a home: triage every point into a todo line, a vault update or a
  reasoned rejection — then delete the dump.

**How work is done → `megaron_arbetssatt.md`** — standing instruction, takes precedence over local habits.
Read it before a slice. Core: contract before code · one slice = one thing · baseline first · judge visuals
at 1:1 · four gates (code/visual/**semantic**/**user**) · **eye-check on the branch before merge** · deploy is
reproduction, not discovery · follow a data invariant through the whole lifecycle · substrate before sibling ·
name the concrete surface where the player meets it · stop and ask Timothy on canon · finish with a proof package.

---

## What this is

Persistent async multiplayer grand strategy, mythic Bronze Age east Mediterranean. 100 **Wanax** per world,
each ruling a network of settlements; kingdoms form organically; the world runs whether you're online or not.
Tone: serious, warm, human-scale. Setting: `temenos_worldbuilding.md`, `temenos_designprinciper.md`.
Current status and backlog: `megaron_todo.md` — do not restate them here.

**Names:** project = **Megaron** (game + web). Server = **Temenos**, CLI = **Keryx**, iOS = **Lawagetas**.
The code sweep is done (module `formatet/megaron/server`, binary `keryx`). **The infra sweep is not** —
`POLEIA_*` env, `poleia_token` cookie, `~/.config/poleia/`, `/var/lib/poleia/`, `/opt/poleia`,
`poleia.service` and the DB name are still live, so **commands in this file stay accurate to reality**.
Runs coordinated with Timothy; see `megaron_namn_hygien.md` §D.

## Stack

Go 1.22+ · chi · PostgreSQL 16 (pgx/v5) · Redis 7 (go-redis) · gorilla/websocket · golang-migrate · log/slog · HTMX + vanilla JS.

- **Event sourcing is hybrid.** `events` is an append-only audit/notify log. **Only loyalty is replay-derived**
  (`settlement/loyalty.go`). Resources, army, silver, kharis and population are mutated with direct `UPDATE`
  on projection tables — **`events` is not the source of truth for them**, so don't plan on rebuilding
  settlement state from the log. Write the events anyway (notifications + audit). Mutate atomically in a TX.
- **Lazy resource eval:** store `(amount, rate_per_minute, calc_at)`, compute on read.
- **Timed event queue** in PostgreSQL (SKIP LOCKED, worker polls every `min(10s, TickSeconds)`).
  WebSocket hub per world for push.

---

## Architecture rules (HARD — do not deviate)

### Time
- **Never call `time.Now()` directly** in game/tick logic. All game time goes through `clock.Clock.Now()`.
  Inject `clock.Clock` via constructor; use `clock.TestClock` in tests. Only `internal/events` and `main.go`
  may hold a `*clock.WallClock`.
- **Sanctioned wall-clock exceptions** (non-game time, deliberately outside `clock.Clock`): WS I/O deadlines
  (`internal/notify/hub.go`) · auth token/cookie expiry (`internal/auth`, `api/handlers/auth.go`) ·
  `internal/chronicle` · `cmd/create-world` seed · CLI display in `cmd/keryx`.
  **No new exceptions without updating this list.**

### Events
- **Idempotency:** every handler registered with `events.Worker` must be safe to run twice. Either
  `SELECT … FOR UPDATE` → work → `UPDATE processed=true` in one transaction, or
  `INSERT … ON CONFLICT (event_id) DO NOTHING` for projection writes. If a handler isn't idempotent, mark it
  `// TODO: idempotent`.
- **Events store outcomes, not intentions.** Probabilistic rolls happen **once** in the handler; the result is
  what goes in the payload. `{"type":"chariot_loss","amount":3}`, never `{"roll_pending":true}`. No event may
  say "check if X happened" — it says "X happened" or doesn't exist.
- **Semantics are frozen forever.** Never change how an existing event type is interpreted; create a new type
  (`MessengerArrivedV2`) instead. Old handlers keep reading old types.
- **Handler timeouts (G2):** `events.Worker` wraps every handler in `context.WithTimeout` (default 5 s), so
  handlers **must** pass `ctx` to every DB call. Three consecutive failures → dead-lettered.

### Package dependency order (G1 — strict, no exceptions)
```
ai, auth, clock, gossip, hexgrid, notify, province, religion, unit, world  ← zero internal deps
  ↑
events(→clock) · tick(→clock,events) · chronicle(→events) · settlement(→province)
  ↑
economy(→clock,events,gossip,hexgrid) · transport(→clock,events,province) · capabilities(→clock,province,religion)
  ↑
kharis(→ai,clock,economy,events,hexgrid,religion,unit) · loyalty(→clock,economy,events,settlement,tick)
  ↑
combat(→…,hexgrid)  ← may use capabilities, economy, gossip, loyalty, province, tick, transport, unit (+clock, events)
  ↑
messenger  ← may use combat + everything below
  ↑
api/handlers, cmd/server  ← may use all (the only ones that may import notify — the hub is consumed
                            via consumer interfaces, e.g. transport.Broadcaster)
```
A package may import **downward only**. Upward communication goes via event emission.
Consumer interfaces are defined in the **consuming** package, never in the implementing one.
(`kingdom` is not a package — kingdoms live in `api/handlers/kingdom.go` + `capabilities/kingdom_verbs.go`,
gated behind `KINGDOMS_ENABLED`.)

### Auth (G3)
Bearer token in the `Authorization` header. (A `poleia_token` cookie exists too, but ONLY for web page
navigation through `auth.WebMiddleware`; all API calls use Bearer.) Wired in
`web/static/js/megaron/api.js` (`fetchAuth`) and via the `htmx:configRequest` listener in
`web/templates/base.html`. iOS will use Keychain → Bearer. No CSRF tokens.

---

## Naming (MUST)

- **Lazy-tuple suffixes:** `*_amount`, `*_rate`, `*_cap`, `*_calc_tick` — NOT `*_last_calc_at`.
- **Silver is a good in `settlement_goods`** (mig 057) — no `silver_*`/`gold_*` columns on settlements
  (sole exception: `sitos_fund_silver`). Prefer **silver** over "gold" for the currency everywhere;
  "gold" is reserved for a future luxury good.
- **Army:** bare names — `infantry`, `chariot`, `ship`, `elite_infantry` (legacy dual-write columns until
  SB7/C8). **`priest` is not a unit** — cult is temple labor.
- **Terminology (use → not):** Wanax not Player · Kharis not Mana · Era not Season · Province not Hex ·
  Settlement not Base · Kingdom not Alliance · Rite not Spell · March not Attack (verb) ·
  Sea Peoples not Boss · Collapse not Season-end · **The Thalassa** not The Sea.
- **The Thalassa** is the sea's in-world lore name and is permanent (`terrain = "sea"` in DB).
  Untouched by the Megaron rename, which is about the system, not the world.

## Design guardrails (respect the SHAPE — numbers live in code/vault)

Get the shape wrong and you write wrong code. Everything else: `megaron_moc.md`.

- **Province ≠ settlement** — separate tables; outpost = province row, no settlement row.
- **Loyalty** — bounded low-integer projection, never 0–100; event-sourced.
- **Kharis** is a relationship, not mana; always a floor (never 0); realm pool per Wanax, cap driven by
  temple level. Cult is produced by population allocated to temples.
- **Messengers are physical and sacred** (uninterceptable); the reply arrives on return. **Load-bearing
  pillar:** ALL info-sharing flows through moving units, and orders to your own units travel by messenger —
  **command is never instant.** Everything else on the map *can* be intercepted.
- **Catchment is the only production source** — the settlement's own hex + a radius-2 ring around it
  (19 hexes, 18 worked; P1, 2026-08-07 — was radius-1/7 hexes before), worked without outposts; dynamic,
  lazy, deterministic. Radius lives in `internal/hexgrid.CatchmentRadius`.
- **Coast is not a terrain** — it's a property (neighbour of sea); `coast_beach` is gone from the enum.
- ⚠️ **Labor = share of pop — REPEALED 2026-08-07 (Timothy), buildings fixed (P2), terrain still
  share-based (P3 not built).** The rule *was*: share of pop (weight semantics), not absolute citizens,
  so growing pop follows the percentage. **P2 (2026-08-07) replaced the building term**:
  `economy.LaborCapacity`'s building contribution is now `economy.WorkplaceSlots(buildingType, level)`,
  an ABSOLUTE headcount per building/level, converted to an effective share via `slots/laborPool` so it
  no longer scales with population. The terrain term (`GoodLaborTerrainBase = 0.25`, no building
  required) is **still** a share of pop — P3 (hex capacity) is what replaces that one, not built yet.
  **Do not build new share-based labour logic for BUILDINGS** — use `WorkplaceSlots`. Terrain-only goods
  are still fair game to leave share-based until P3. Rewrite this bullet when P3 lands; plan and phases:
  `megaron_plan_fysisk_gubbemodell.md` (vault), design in `Temenos_varutaxonomi_sol.md` §1.1, §8.
- **Cost ↔ upkeep** — upkeep = grain+silver ∝ build cost. Strategic metals belong in build gates, recruit
  and attrition, **never flat upkeep** (bronze upkeep = desertion spiral).
- **Trade & messenger layer — three distinct things, keep them apart:** (1) **message** = free text
  wanax↔wanax; (2) **trade offer** = structured, bilateral consent, **FOW-gated to cities you have actually
  contacted** (`visibleOrigins`) — no global trade catalogue; (3) **internal transfer** own→own settlement =
  logistics, no consent, a physical caravan that does not roll the loss die.
- **Kingdoms are POST-MVP** (Timothy 2026-07-08): all player surface disabled; server code kept gated.

## Visual style (palette lives in code)

- Use the CSS custom properties in `web/static/megaron.css` `:root`. **Never** hardcode hex in
  templates/CSS and never inline `style="color:#..."` — add a class to `megaron.css`.
- Pixel art: 1px CHARCOAL outline on solids; no anti-aliasing, gradients or rounded corners; background
  terrain desaturated, foreground objects saturated.
- The canvas renderer is exempt from the CSS vars (its own internal palette; culture accents live there).
- Full spec: `temenos_designprinciper.md`. Rendering principles: `megaron_terrangrendering.md`.

## Running it

- **Local:** `docker compose up` at project root (migrations run on startup; copy `.env.example` → `.env`).
- **Dev server** (CT 126, 10.0.1.92:8080) runs `air`. After pushing to master:
  `! ssh root@10.0.1.92 "cd /opt/poleia && git pull && echo done"` — `air` rebuilds and applies
  migrations by itself seconds later. **Don't restart reflexively**; only when `air` is wedged.
  ⚠️ **`air` watches `.go` only** (`include_ext = ["go"]`), so a commit whose only production
  artefact is a `.sql` migration **never deploys itself** — it needs `systemctl restart poleia`.
  Same for `base.html` edits. (Found 2026-08-02 with mig 105: pulled, but the process sat unchanged
  for 35 min and the migration never ran.)
  ⛔ **Never restart, and never kill DB sessions, while a migration is in flight.** On live data a
  migration can run for many minutes and that is normal, not a hang. Interrupting one leaves
  `schema_migrations.dirty` set and the server down. Wait it out; if you must diagnose, read only.
  Deploy is verified by commit hash + **ground state** + `healthz`, never `schema_migrations` alone.
  Timings, DB sizes and the retention debt live in `megaron_drift.md`.
- **Isolated acceptance world:** `tools/acceptance.sh` — this is where eye-checks happen, before merge.
- **`keryx` binary:** `~/go/bin/poleia` — NOT in PATH, always use the full path.
- **LLM playtest agents + live world:** `keryx_playtest.md`.
