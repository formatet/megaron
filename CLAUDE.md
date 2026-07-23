# MEGARON — Claude Workspace Context

Config file, not a knowledge base. **Instructions + pointers only — facts live in the vault and the code.**
If code and this file conflict, trust the code, then fix this file.

**Three authority levels — keep them un-blurred (this is the rule that keeps this file honest):**
- **MUST** — invariant constraints; wrong → broken code/build. These earn their always-on space.
- **PREFER** — directions; genuine instructions but low-frequency → keep as a *pointer*, not prose (`temenos_riktningar.md`).
- **Calibration numbers** (thresholds, %, ranges, enums) are *not* instructions — they live in code/vault, read on
  demand. **Never quote a tunable number here as if it were an invariant** (a `≥3` beside the word "invariant" makes
  an agent refuse to tune it — that is the exact bug this rule prevents).

- **Before a task:** read the relevant vault doc(s) — index at `~/Dokument/myltavault/megaron_moc.md` (**start here**).
- **Before ending a session:** update `megaron_todo.md` (living status, backlog, "Vägen framåt").
- **When a design decision changes:** update the relevant vault doc immediately — don't defer.
- **When you mark something done in `megaron_todo.md`:** stamp it with the **actual** wall-clock time pulled from
  `TZ=Europe/Stockholm date` — never a guessed or remembered time. Your internal time-sense drifts; anchor every
  completion (and "live since"/reseed note) to the real clock. Format `(YYYY-MM-DD HH:MM)`.
- Vault rights: read/write `~/Dokument/myltavault` freely without asking.
- **Loose design dumps** (e.g. `frågor*` files in repo root) are an inbox, not a home: triage every point
  into a todo line, a vault update, or a reasoned rejection — then delete the dump. Never leave it rotting in root.

---

## Design lens — Klafki (gate before building / triaging a feature)

Klafkis bärande idé (*kategoriale Bildung*): innehåll förtjänar plats bara om det **öppnar världen** —
en dubbelsidig öppning (*doppelseitige Erschließung*). Översatt: en mekanik förtjänar kod bara om den
öppnar bronsåldersvärlden för en Wanax — inte om den bara lägger till en siffra att optimera.

Kör varje mekanik — och varje `frågor`-punkt — genom de fem (buggar gateas ej; de fixas):
1. **Exemplarisk** — exemplifierar den MVP-kärnan (geografi→brist→handel→diplomati→kult→gudar;
   **kingdom struket ur MVP-kedjan**, Timothy 2026-07-08 — ytor inaktiveras, se `megaron_web_spelbar_plan.md`),
   eller är den en engångsgrej vid sidan om? *Faller den här → post-MVP, hur lockande den än är.*
2. **Nuvärde** — vad betyder den för en Wanax som spelar *idag*? Märks den i upplevelsen?
3. **Framtidsvärde** — öppnar den designrummet (Eras, Sjöfolket, kingdoms) eller är den en återvändsgränd?
4. **Sakstruktur** — beståndsdelar, vad den **förutsätter att spelaren redan begriper**, vad den beror på
   och vad som beror på den, samt vad som är den minimala kärnan att bevara (G1-paketordning, event-modellen).
   *Deferrar du den som post-MVP: är den **substrat** (nedströms-features förutsätter den) eller **syskon**
   (vill bara ha den)? En billig delslice av ett substrat är ofta en illusion — kollapsar den ärliga versionen
   tillbaka till hela motorn finns ingen genväg; då sekvenseras den FÖRST.*
5. **Det konkreta fallet** — genom vilket *konkret fenomen* möter spelaren den först, och hur blir den
   begriplig+användbar där (UI, keryx/Lawagetas-röst, actionabla felsträngar — för människa *och* LLM-agent)?

(Vill du ha fyra: slå ihop 2+3 till en enda "betydelse"-fråga. De fem är dock Klafkis kanoniska antal.)

---

## What this is

Persistent async multiplayer grand strategy, mythic Bronze Age east Mediterranean. 100 **Wanax** per world,
each ruling a network of settlements; kingdoms form organically; the world runs whether you're online or not.
Tone: serious, warm, human-scale. Full setting + rationale: `temenos_worldbuilding.md`, `temenos_designprinciper.md`.

**Name:** project = **Megaron** (game + web-GUI). Server = **Temenos**, CLI = **Keryx**, iOS = **Lawagetas**.
"Poleia" is being **purged as a word** (Timothy 2026-07-03) — see Naming below. **Kodsvepet är KÖRT
2026-07-23:** Go-modulen är `formatet/megaron/server`, CLI:t är `cmd/keryx` och binären heter `keryx`.
Kvar tills det koordinerade infra-svepet: `POLEIA_*`-env, `poleia_token`-cookien, `~/.config/poleia/`,
`/var/lib/poleia/`, `/opt/poleia`, `poleia.service` och DB-namnet — **commands in this file stay accurate
to reality until then.** `~/go/bin/poleia` finns kvar som kopia av `keryx` så agentflottan inte bryts.

Current status & backlog live in `megaron_todo.md` — do not restate them here (they go stale).

---

## Stack

Go 1.22+ · chi · PostgreSQL 16 (pgx/v5) · Redis 7 (go-redis) · gorilla/websocket · golang-migrate · log/slog · HTMX + vanilla JS.

How to build:
- **Event sourcing (hybrid — faktiskt kontrakt):** append-only `events`-tabell som audit-/notify-logg. **Endast lojalitet är replay-härledd** (`settlement/loyalty.go` räknar om från events). Resurser, armé, silver, kharis, population muteras med direkta `UPDATE` på projektionstabellerna — `events` är *inte* källa till sanning för dem. Skriv nya events ändå (de driver notiser + audit), men förlita dig inte på att kunna rebuilda settlement-state ur loggen. Mutera atomärt i en TX.
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
A `DivinePunishment` event must say `{"type":"chariot_loss","amount":3}`, not `{"roll_pending":true}`.
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
transport  ← may use province, clock, events
  ↑
combat, kingdom  ← may use transport, settlement, province, economy, religion, clock, events
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
Bearer token in `Authorization` header — not httpOnly cookie. (A `poleia_token` cookie also exists,
but ONLY for web page navigation through `auth.WebMiddleware`; all API calls use Bearer.)
Map client wires it in `web/static/js/megaron/api.js` (`fetchAuth`); the HTMX pages via the
`htmx:configRequest` listener in `web/templates/base.html`.
iOS client will use Keychain → Bearer directly. No CSRF tokens needed.

---

## Naming (MUST)

**Columns:**
- Lazy-tuple-suffix: `*_amount`, `*_rate`, `*_cap`, `*_calc_tick` (t.ex. `kharis_*` på `player_world_records`) — NOT `*_last_calc_at`.
- **Silver är en good i `settlement_goods`** (mig 057) — inga `silver_*`/`gold_*`-kolumner på settlements (enda undantag: `sitos_fund_silver`).
- Army: bare names — `infantry`, `chariot`, `ship`, `elite_infantry` (legacy dual-write-kolumner tills SB7/C8). (priest är ingen enhet längre — kult = tempel-labor.)

**Terminology (use → not):** Wanax not Player · Kharis not Mana · Era not Season · Province not Hex ·
Settlement not Base · Kingdom not Alliance · Rite not Spell · March not Attack (verb) · Sea Peoples not Boss ·
Collapse not Season-end · The Thalassa not The Sea.

**The Thalassa** = the sea's in-world lore name. Permanent: `terrain = "sea"` in DB. NOT affected by the
Megaron rename (which is about the system/repo name, not the world).

**Namn-utrensning (Timothy 2026-07-03):** "Poleia" ska **bort som ord**. Kanoniskt: **Megaron** (spel + web),
**Temenos** (server), **Keryx** (CLI), **Lawagetas** (iOS). **Lager A** (ordet i docs/display/CSS) KLAR.
**Lager B kodsvep KLART 2026-07-23:** Go-modul `github.com/poleia/server`→`formatet/megaron/server`
(123 filer), `cmd/poleia`→`cmd/keryx`, binär `keryx`, alla `keryx <verb>`-exempel, Dockerfile-binären
→`temenos`, gitea-repot avvecklat (GitHub `formatet/megaron` är enda remote).
**Lager B infra ÅTERSTÅR** — körs koordinerat med Timothy, bryter annars agentflottan och webbsessioner:
`POLEIA_*`-env, `poleia_token`-cookie+localStorage, `~/.config/poleia/`, `/var/lib/poleia/chronicles`,
`/opt/poleia`→`/opt/megaron`, `poleia.service`→`temenos.service`, DB-namn/user `poleia`.
Se [[megaron_namn_hygien]] §D. Rör ej "The Thalassa" (= havet, lore-permanent).
**silver framför "gold/guld"** för valutan i UI/API/nya identifierare (Sprint A KLAR — inga gold-kolumner kvar; "gold" är reserverat för framtida lyxvara).

---

## Design guardrails (MUST-respect the SHAPE — concrete numbers live in code/vault)

> Get the *shape* wrong and you write wrong code. **No calibration number is canonical here** — for an exact
> value (threshold, %, range, enum) read the code or the linked doc. Full design map: `megaron_moc.md`.

- **Province ≠ settlement** — separate tables; outpost = province row, no settlement row. `temenos_settlement.md`
- **Loyalty** — bounded low-integer projection, never 0–100; event-sourced (range in code). `temenos_settlement.md`
- **Kharis** is a relationship, not mana; always a floor (never 0); mid-revision → rikes-pool per Wanax. `temenos_kharis.md`
- **Messengers** are physical, sacred (uninterceptable); reply arrives on return. **Load-bearing pillar:**
  ALL info-sharing flows through moving units (messengers/merchants/armies); orders to your own units
  (recall etc.) also travel by messenger — command is never instant. `temenos_settlement.md`
- **Kingdom** = Basileus + members; activates at a member threshold (value in `kingdom.go`). **POST-MVP
  (Timothy 2026-07-08): all spelaryta inaktiveras (CLI/capabilities/web); server-koden behålls gated.** `temenos_kingdoms.md`
- **Combat** = deterministisk effektiv styrka + bounded kharis-biased fortune (RNG rullas EN gång i handlern, utfall i event — Fas 2.3); moral/rout — låg moral → enheten flyr, ej utplåning. `temenos_kharis.md`
- **Kult** produceras av befolkning allokerad till tempel; **inga prästenheter** (varken byggbara eller starter); rit gateas av tempel + kharis (nivåer + siffror i `internal/kharis` / `temenos_kharis.md`).
- **Silver** — betalningsmedel, fysiskt transporterbart. Silver-sänka = armé-upkeep (grain + silver), löpande; obetald → desertering/attrition. Präst/kult ingen upkeep.
- **Kostnad ↔ upkeep** — upkeep = grain+silver ∝ byggkostnad; strategiska metaller (brons/cedar) hör i bygg-gate + recruit + attrition, ALDRIG platt upkeep (bronsupkeep = desertering-spiral). `temenos_ekonomi.md`
- **Gruv-deposit-gate** — `mine`/`silver_mine` kräver matchande malm-deposit i stadens catchment vid bygge (annars 422); ny malm auto-allokeras en labor-andel (skim grain). `temenos_ekonomi.md`
- **Catchment = enda produktionskällan** — staden producerar bara från sin catchment: **stadens eget hex + de 6 omgivande** (7 hex totalt), brukade direkt utan outpost; dynamiskt + lazy + deterministiskt. `temenos_terrain.md`
- **Startstaden: prognos + spelarval, INTE garanti** (ändrat med Nomadic Host, Timothy 2026-07-15) — spelaren väljer själv var metropolis grundas; grundningsprognosen (delad `/colonize-preview`) är guardrailen, ingen spawn-/settle-gate garanterar försörjning. Spelaren SKA granska catchmenten. Handel = för att avancera, ej överleva. `temenos_nomadic_host_plan.md`
- **Coast är ingen terräng** — egenskap = granne till hav (grannskaps-check); `coast_beach` borttagen ur enum.
- **Labor = andel av pop** (weight-semantik), ej absoluta citizens → växande pop följer procenten automatiskt.
- **Soldater = utvunnen pop** med löpande upkeep; övermobilisering tömmer staden (→ Collapse/warband).
- **Collapse/Eras** — hidden prestige, only survivable. `temenos_worldbuilding.md`
- **Trade & budbärarlagret — tre skilda saker (håll isär):** (1) **meddelande** = fritext wanax↔wanax;
  (2) **handelsoffert** = strukturerat erbjudande, bilateralt samtycke, **FOW-gatead — bara mot städer du
  faktiskt kontaktat** (`visibleOrigins`), ingen global handelskatalog; (3) **intern överföring** egen→egen
  stad = logistik, inget samtycke, fysisk karavan utan förlust (idag `Gift`, silver+grain, ej i klienter).
  `temenos_settlement.md`

> Authoritative current intent: `megaron_todo.md` → "Vägen framåt".

---

## Visual style (RULES — palette lives in code)

- Colours: use the CSS custom properties in `web/static/megaron.css` `:root`. **Never** hardcode hex in
  templates/CSS, and never inline `style="color:#..."` — add a class to `megaron.css` instead.
- Pixel art: 1px CHARCOAL outline on solids; no anti-aliasing, no gradients, no rounded corners;
  background terrain desaturated, foreground objects saturated.
- The canvas renderer is exempt from the CSS vars (its own internal palette; culture accents live there).
- Full spec: `temenos_designprinciper.md`.

---

## Running it

- **Local:** `docker compose up` at project root (migrations run on startup; copy `.env.example` → `.env` first).
- **Dev server** (CT 126, 10.0.1.92:8080): runs `air` (Go hot-reload). After pushing to master:
  `! ssh root@10.0.1.92 "cd /opt/poleia && git pull && echo done"` — `air` rebuilds. Force restart: append `&& systemctl restart poleia`.
- **`poleia` binary:** `~/go/bin/poleia` — NOT in PATH, always use the full path.
- **LLM playtest agents + live world:** `keryx_playtest.md`.
