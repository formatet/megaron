# Megaron

A persistent asynchronous multiplayer strategy game set in the mythic Bronze Age eastern Mediterranean.

You are a **Wanax**. You lead a nomadic host to a coast worth settling, found a metropolis, and grow a network of settlements: work the land, trade for the metals you lack, send messengers to your neighbours, court capricious gods, and survive until the Sea Peoples come. The world runs whether you are online or not.

Nothing is instant. Every message, every trade, every army travels as a physical unit across the map — including the orders you send to your own troops.

**Inspirations:** Utopia (1998) · Settlers 2 · Europa Universalis 4 · Crusader Kings · Machiavelli: The Merchant Prince · Sid Meier's Colonization · Diplomacy (board game)

---

## The pieces

| Name | What it is |
|------|------------|
| **Megaron** | The project — game + web client |
| **Temenos** | The server |
| **Keryx** | The command-line client (`server/cmd/keryx`) |
| **Lawagetas** | The iOS client (planned) |
| **The Thalassa** | The sea, in world-lore |

---

## Run locally (3 commands)

```bash
git clone <this-repo> && cd megaron
cp .env.example .env          # edit JWT_SECRET at minimum
docker compose up
```

Open http://localhost:8080 — register, create a world, join it. Migrations run on startup.

### The CLI

Everything the server exposes is reachable from `keryx` — that is a project rule, not a nicety.

```bash
cd server && go build -o ~/go/bin/keryx ./cmd/keryx
keryx login && keryx status
```

---

## Self-hosting a world

1. Install Docker and Docker Compose on any Linux server.
2. Copy the repo (or just `docker-compose.yml` + `.env.example`).
3. Set `JWT_SECRET` in `.env` to something long and random.
4. `docker compose up -d`
5. Share your server's address with players — they register and join your world.

The server runs migrations, generates the map, and processes timed events (army arrivals, build completions, upkeep, tithes) on its own. No cron jobs needed.

---

## Configuration

All environment variables — required and optional — are documented with their code-side defaults in **[`.env.example`](.env.example)**. The essentials:

| Variable       | Default          | Description                                     |
|----------------|------------------|-------------------------------------------------|
| `DATABASE_URL` | (set in compose) | PostgreSQL connection string                    |
| `REDIS_URL`    | `redis:6379`     | Redis address (host:port)                       |
| `JWT_SECRET`   | **required**     | HS256 signing secret — keep private             |
| `PORT`         | `8080`           | HTTP listen port                                |
| `TICK_SECONDS` | 60 min/tick      | Tick cadence; set low (e.g. `6`) to speed up dev |

Tuning knobs for the Sitos fund, map size, kingdoms and admin endpoints live in `.env.example`.

---

## API

All routes are under `/api/v1`; authentication is a **Bearer token** in the `Authorization` header. Live WebSocket push is at `/ws/{worldID}`.

```
POST  /auth/register                             { username, email, password }
POST  /auth/login                                { username_or_email, password }
GET   /auth/me                                   → player info

GET   /worlds                                    → world list
GET   /worlds/:id                                → world + collapse state
GET   /worlds/:id/map                            → fog-of-war hex tiles
GET   /worlds/:id/colonize-preview               → catchment forecast for a candidate site
POST  /worlds/:id/join                           → begin as a nomadic host
POST  /worlds/:id/founding/settle                → found your metropolis where the host stands

GET   /worlds/:id/provinces/:pid                 → settlement detail + live resources
GET   /worlds/:id/provinces/:pid/goods           → lazily-evaluated stores
POST  /worlds/:id/provinces/:pid/build           → queue a building
POST  /worlds/:id/provinces/:pid/recruit         → raise a unit
PUT   /worlds/:id/provinces/:pid/labor           → reallocate population
POST  /worlds/:id/provinces/:pid/trade           → offer a trade (fog-of-war gated)
POST  /worlds/:id/provinces/:pid/march           → send an army { target_id, intent, … }

GET   /worlds/:id/marches | /messengers | /trades  → units in transit
GET   /worlds/:id/market/wants                   → what your contacts are short of
```

Kingdoms (`/worlds/:id/kingdoms/*`) exist server-side but are **post-MVP** and gated behind `KINGDOMS_ENABLED`; unset, they answer `403 kingdoms_disabled`.

---

## Architecture

- **Go + chi** — HTTP server, graceful shutdown
- **PostgreSQL 16** (pgx/v5) — projections + a durable job queue (`SKIP LOCKED`)
- **Redis 7** — sessions and pub/sub
- **HTMX + vanilla JS** — web frontend, no framework; the hex map is a canvas renderer
- **Hybrid event sourcing** — an append-only `events` table drives notifications and audit. Only *loyalty* is replay-derived; other state is projected and mutated directly inside a transaction. Handlers are idempotent, and events record **outcomes**, never intentions.
- **Lazy resource evaluation** — stored as `(amount, rate, calc_tick)`, computed on read
- **Combat** is a deterministic strength calculation plus a bounded, kharis-biased fortune roll (±0.2) made **once** in the handler; low morale routs a unit rather than annihilating it
- **Timed events** in `scheduled_events`, polled by a worker (10 s default, faster when ticks are short), with per-handler timeouts and dead-lettering
- **Strict package dependency order** — packages import downward only; upward communication goes through event emission
- Each world can be self-hosted independently via Docker

Design documents, current status and backlog live outside this repo, in the author's vault and the project wiki.

---

## License

See [LICENSE](LICENSE).
