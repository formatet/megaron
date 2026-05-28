# Thalassa

A persistent asynchronous multiplayer strategy game set in the mythic Bronze Age eastern Mediterranean.

Each player governs a network of settlements: build, recruit armies, form kingdoms, court capricious gods, and survive until the Sea Peoples come. The world runs whether you are online or not.

**Inspirations:** Utopia (1998) · Settlers 2 · Europa Universalis 4 · Crusader Kings · Machiavelli: The Merchant Prince · Sid Meier's Colonization · Diplomacy (board game)

---

## Run locally (3 commands)

```bash
git clone <this-repo> && cd thalassa
cp .env.example .env          # edit JWT_SECRET at minimum
docker compose up
```

Open http://localhost:8080 — register, create a world, join it.

---

## Self-hosting a world

1. Install Docker and Docker Compose on any Linux server.
2. Copy the repo (or just `docker-compose.yml` + `.env.example`).
3. Set `JWT_SECRET` in `.env` to something long and random.
4. `docker compose up -d`
5. Share your server's address with players — they register and join your world.

That's it. The server runs migrations on startup, generates maps, and processes timed events (army arrivals, build completions) automatically. No cron jobs needed.

---

## Environment variables

| Variable        | Default                          | Description                          |
|-----------------|----------------------------------|--------------------------------------|
| `DATABASE_URL`  | (set in compose)                 | PostgreSQL connection string         |
| `REDIS_URL`     | `redis:6379`                     | Redis address (host:port)            |
| `JWT_SECRET`    | **required**                     | HS256 signing secret — keep private  |
| `PORT`          | `8080`                           | HTTP listen port                     |
| `STATIC_DIR`    | `../../web/static`               | Path to static assets                |
| `TEMPLATE_DIR`  | `../../web/templates`            | Path to HTML templates               |

---

## API quick reference

All API routes are under `/api/v1`.

```
POST  /auth/register          { username, email, password }
POST  /auth/login             { username_or_email, password }
GET   /auth/me                → player info

GET   /worlds                 → world list
POST  /worlds                 create world (authenticated)
GET   /worlds/:id             → world + collapse state
GET   /worlds/:id/map         → fog-of-war hex tiles (JSON)
POST  /worlds/:id/join        → create your province, returns province_id

GET   /worlds/:id/provinces/:pid       → province detail + live resources
POST  /worlds/:id/provinces/:pid/march → send army { target_id, intent, infantry, … }

GET   /worlds/:id/kingdoms             → kingdom list
POST  /worlds/:id/kingdoms             → found kingdom { name }
POST  /worlds/:id/kingdoms/:kid/invite → invite province { province_id }
POST  /worlds/:id/kingdoms/:kid/join   → accept invitation
```

---

## Architecture

- **Go + chi** — HTTP server, graceful shutdown
- **PostgreSQL** — event-sourced game state, durable job queue
- **Redis** — sessions, pub-sub (WebSocket hub, coming)
- **HTMX** — web frontend, no JS framework
- Resources use **lazy evaluation** — stored as `(amount, rate, timestamp)`, computed on read
- Combat is **deterministic** — no randomness, formulas are public
- **Timed events** (army arrivals, builds) stored in `scheduled_events`, processed every 10 s
- Each world can be self-hosted independently via Docker

---

## Sprint status

- ✅ Auth (register, login, JWT + cookies)
- ✅ World creation + procedural hex map generation
- ✅ Province creation (join world → tile assigned by terrain)
- ✅ Lazy resource calculation (6 resources)
- ✅ Army marching + deterministic combat resolver
- ✅ Kingdom system (found, invite, join, council roles)
- ✅ Religion model (temples, divine intervention, pantheon power by geography)
- ✅ Collapse system (era-week + prestige + active wars)
- ✅ Scheduled event worker
- ✅ HTMX web frontend (login, worlds, province view, hex map, kingdom)
- 🔲 WebSocket real-time push
- 🔲 Army arrival → combat resolution (worker handler)
- 🔲 Build queue processing
- 🔲 Divine intervention rolls
