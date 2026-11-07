#!/usr/bin/env bash
# Prövar mig 248 på en värld med 40 stormar vid tick 1000: dies_tick ska vara spridd (inte konstant),
# worlds.storm_weather_tick ska backfyllas, och down ska gå. Körs från repo-roten. Port: $PORT (56913).
set -euo pipefail
PORT="${PORT:-56913}"; N=mig248-$$; trap 'docker rm -fv $N >/dev/null 2>&1 || true' EXIT
docker run -d --rm --name $N -e POSTGRES_PASSWORD=pw -p $PORT:5432 postgres:16-alpine >/dev/null
for _ in $(seq 30); do docker exec $N pg_isready -h 127.0.0.1 -U postgres -q 2>/dev/null && break; sleep 1; done
docker exec $N psql -U postgres -qc 'CREATE DATABASE m;'
DSN="postgres://postgres:pw@localhost:$PORT/m?sslmode=disable"; P="docker exec $N psql -U postgres -d m -tAc"
(cd server && migrate -path db/migrations -database "$DSN" goto 247 >/dev/null 2>&1)
$P "INSERT INTO worlds(name,status,current_tick) VALUES('w','archived',1000)" >/dev/null
$P "INSERT INTO sea_storms(world_id,heading,created_tick) SELECT id,0,900 FROM worlds, generate_series(1,40)" >/dev/null
$P "INSERT INTO sea_storm_track SELECT id,1000,0,0,0 FROM sea_storms" >/dev/null
(cd server && migrate -path db/migrations -database "$DSN" up 2>&1 | tail -1)
$P "SELECT 'distinct='||count(DISTINCT dies_tick)||' min='||min(dies_tick)||' max='||max(dies_tick) FROM sea_storms"
$P "SELECT 'storm_weather_tick='||storm_weather_tick FROM worlds"
(cd server && migrate -path db/migrations -database "$DSN" down 1 2>&1 | tail -1)
