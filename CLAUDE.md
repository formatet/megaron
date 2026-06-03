# POLEIA — Claude Workspace Context

Config file, not a knowledge base. **Instructions + pointers only — facts live in the vault and the code.**
If code and this file conflict, trust the code, then fix this file.

- **Before a task:** read the relevant vault doc(s) — index at `~/Dokument/myltavault/poleia_index.md` (**start here**).
- **Before ending a session:** update `thalassa_todo.md` (living status, backlog, "Vägen framåt").
- **When a design decision changes:** update the relevant vault doc immediately — don't defer.
- Vault rights: read/write `~/Dokument/myltavault` freely without asking.
- **Loose design dumps** (e.g. `frågor*` files in repo root) are an inbox, not a home: triage every point
  into a todo line, a vault update, or a reasoned rejection — then delete the dump. Never leave it rotting in root.

---

## Design lens — Klafki (gate before building / triaging a feature)

Klafkis bärande idé (*kategoriale Bildung*): innehåll förtjänar plats bara om det **öppnar världen** —
en dubbelsidig öppning (*doppelseitige Erschließung*). Översatt: en mekanik förtjänar kod bara om den
öppnar bronsåldersvärlden för en Wanax — inte om den bara lägger till en siffra att optimera.

Kör varje mekanik — och varje `frågor`-punkt — genom de fem (buggar gateas ej; de fixas):
1. **Exemplarisk** — exemplifierar den MVP-kärnan (geografi→brist→handel→diplomati→kingdom→kult→gudar),
   eller är den en engångsgrej vid sidan om? *Faller den här → post-MVP, hur lockande den än är.*
2. **Nuvärde** — vad betyder den för en Wanax som spelar *idag*? Märks den i upplevelsen?
3. **Framtidsvärde** — öppnar den designrummet (Eras, Sjöfolket, kingdoms) eller är den en återvändsgränd?
4. **Sakstruktur** — beståndsdelar, vad den **förutsätter att spelaren redan begriper**, vad den beror på
   och vad som beror på den, samt vad som är den minimala kärnan att bevara (G1-paketordning, event-modellen).
5. **Det konkreta fallet** — genom vilket *konkret fenomen* möter spelaren den först, och hur blir den
   begriplig+användbar där (UI, keryx/Lawagetas-röst, actionabla felsträngar — för människa *och* LLM-agent)?

(Vill du ha fyra: slå ihop 2+3 till en enda "betydelse"-fråga. De fem är dock Klafkis kanoniska antal.)

---

## What this is

Persistent async multiplayer grand strategy, mythic Bronze Age east Mediterranean. 100 **Wanax** per world,
each ruling a network of settlements; kingdoms form organically; the world runs whether you're online or not.
Tone: serious, warm, human-scale. Full setting + rationale: `thalassa_worldbuilding.md`, `thalassa_designprinciper.md`.

**Name:** project = **Poleia**; the sea = **The Thalassa**. Permanent: `terrain = "sea"` in DB, "The Thalassa" in UI/labels.

Current status & backlog live in `thalassa_todo.md` — do not restate them here (they go stale).

---

## Stack

Go 1.22+ · chi · PostgreSQL 16 (pgx/v5) · Redis 7 (go-redis) · gorilla/websocket · golang-migrate · log/slog · HTMX + vanilla JS.

How to build:
- **Event sourcing:** append-only `events` table — never UPDATE game state directly; projection tables are computed views.
- **Lazy resource eval:** store `(amount, rate_per_minute, calc_at)`, compute on read.
- **Timed event queue** in PostgreSQL (SKIP LOCKED, worker polls every 10s). WebSocket hub per world for push.

---

## Architecture rules (HARD — do not deviate)

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

---

## Naming (HARD)

**Columns:**
- Resources: `gold_amount`, `gold_rate`, `gold_cap`, `gold_calc_at` — NOT `*_last_calc_at`.
- Army: bare names — `infantry`, `cavalry`, `catapult`, `priest`, `ship`, `elite_infantry`.

**Terminology (use → not):** Wanax not Player · Kharis not Mana · Era not Season · Province not Hex ·
Settlement not Base · Kingdom not Alliance · Rite not Spell · March not Attack (verb) · Sea Peoples not Boss ·
Collapse not Season-end · The Thalassa not The Sea.

**Megaron-övergång (HARD, riktning):** Vi fasar **sakta** ut orden "Thalassa" och "Poleia" till förmån för
**Megaron** (systemnamnet). Inget tvångsbyte i en sweep — men varje gång du ändå rör en yta där orden
står (UI-text, ny kod, rubriker), välj Megaron-nomenklaturen. Permanent kvar: `terrain = "sea"` i DB.
Mono-repo-målet temenos/megaron/keryx, se [[thalassa_todo]] "Projektbyte".

---

## Design invariants (one-liners — rationale & full design in vault)

- **Province ≠ settlement** — separate tables; outpost = province row, no settlement row. `thalassa_settlement.md`
- **Loyalty 1–4 only**, never 0–100; event-sourced projection. `thalassa_settlement.md`
- **Kharis** is a relationship, not mana; 5% floor always; mid-revision → rikes-pool per Wanax. `thalassa_kharis.md`
- **Messengers** are physical, sacred (uninterceptable); reply arrives on return. **Load-bearing pillar:**
  ALL info-sharing flows through moving units (messengers/merchants/armies); orders to your own units
  (recall etc.) also travel by messenger — command is never instant. `thalassa_settlement.md`
- **Kingdom** = Basileus + members; forming until 3 members; elections Sundays, 7-day lock. `thalassa_kingdoms.md`
- **Combat** deterministic, no dice; walls L0–3 = 1.0 / 1.25 / 1.5 / 1.75×; priests give 0 field strength. `internal/combat`
- **Priests** — rituella enheter, ingen stridsstyrka. Kharis avgör rit-framgång (80/50/20/5% per mood).
- **Silver** — betalningsmedel (inte guld). DB-nyckel: `gold`. UI-visning: shekel/mina/talang. Fysiskt transporterbart.
- **Collapse/Eras** — hidden prestige, risk from week 10, only survivable. `thalassa_worldbuilding.md`
- **Trade** — bilateral samtycke via budbärare. Intern resursflöde via /trade (egna settlements). extern = messenger+handelsoffert.

> Authoritative current intent: `thalassa_todo.md` → "Vägen framåt".

---

## Visual style (RULES — palette lives in code)

- Colours: use the CSS custom properties in `web/static/poleia.css` `:root`. **Never** hardcode hex in
  templates/CSS, and never inline `style="color:#..."` — add a class to `poleia.css` instead.
- Pixel art: 1px CHARCOAL outline on solids; no anti-aliasing, no gradients, no rounded corners;
  background terrain desaturated, foreground objects saturated.
- The canvas renderer is exempt from the CSS vars (its own internal palette; culture accents live there).
- Full spec: `thalassa_designprinciper.md`.

---

## Running it

- **Local:** `docker compose up` at project root (migrations run on startup; copy `.env.example` → `.env` first).
- **Dev server** (CT 126, 10.0.1.88:8080): runs `air` (Go hot-reload). After pushing to master:
  `! ssh root@10.0.1.88 "cd /opt/poleia && git pull && echo done"` — `air` rebuilds. Force restart: append `&& systemctl restart poleia`.
- **`poleia` binary:** `~/go/bin/poleia` — NOT in PATH, always use the full path.
- **LLM playtest agents + live world:** `poleia_playtest.md`.
