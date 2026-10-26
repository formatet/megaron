# Täckning och egna mutationer

Nio fysiska assertion-röda mutanter på slutkoden `0d5bc55f`, i separat clean checkout. Varje arm använder tools/gotest.sh med egen tom216DB och ren miljö. Ingen compilefail räknad. Exakt återställning till SHA256 i mutations/verified.json, sedan hela authpaketet färskt grönt. Första körningens portkollision underkändes; hela matrisen kördes om med explicita lediga portar, se mutations-port-collision.log och mutations.log.

| Prov | Täcker | Täcker inte |
|---|---|---|
| TestAuthenticatedActivityLifecycle | riktigPG, API/webcookie/optional, ogiltigJWT ingetstamp, första kontakt, cachedrequest ingetDBanrop/ingenrowwrite, separatprocess femminutersSQLgate, sexminuteruppdatering, authidentitet bevarad | saknadtoken på alla routes (API401 finns äldreprov), alla publika handlers, människans uppmärksamhet |
| TestActivityConcurrentRequestsWriteOnce |20samtidiga separataserviceinstanser, riktigUPDATE-trigger räknar exakt1rowupdate | multipla fysiska appservrar eller lasttest |
| TestActivityFailureDoesNotRejectAuthentication | avbrutenrequestcontext ger lyckadauth, ingenstamp, nästa lyckadeobservation retryas | verkligPGoutage, varje möjlig driverfailure; timeout är kodbegränsad1s |
| TestActivityFiveMinuteGateAndCacheExpiry |299sekunder ingenUPDATE,301sekunder + expiredcache nyobservation, lazycacheeviction körs | exakt mikrosekundgräns300 eller wallclockbakåt |
| player_activity_acceptance.py | riktig register/API-Bearer, cookienavigering /play, nativeKeryx unitlist, NULLföreförstakontakt, inga extrafält i /auth/me, upprepning samma xmin; offline216→215→216 bevarar konton och återinförNULL | visuella UIändringar finns inte, WS-optional över socket, verklig spelaraktivitet; schema-rollback släpper avsiktligt observationerna |
| Pompejus audit + journal | sammaworldID, sparad sent_at/tick, journalens faktiska tickgränser379–382 | annan budfysik; ingen grund finns för sådan fix här |

Ingen ny spelhandling eller offentlig spelaryta införs, så ingen ny Keryx-/webb-/Codexknapp behövs. Detta är authtelemetri och ett read-only driftverktyg.

## Exakta GNU sed-mutanter

Kör på separat clean checkout, återställ filen mellan varje mutation. Alla rader nedan verifierades att producera exakt samma bytes som det körda skriptet. Råloggar ligger i mutations/.

### middleware-api

```sh
sed -z -E -i 's#svc\.recordActivity\(r\.Context\(\),\ claims\.PlayerID\)#// activity observation removed#1' server/internal/auth/middleware.go
```

Förväntat och faktiskt assertion-rött: `TestAuthenticatedActivityLifecycle`. [Rålogg](mutations/middleware-api.log).

### middleware-web

```sh
sed -z -E -i 's#svc\.recordActivity\(r\.Context\(\),\ claims\.PlayerID\)#// activity observation removed#2' server/internal/auth/middleware.go
```

Förväntat och faktiskt assertion-rött: `TestAuthenticatedActivityLifecycle`. [Rålogg](mutations/middleware-web.log).

### middleware-optional

```sh
sed -z -E -i 's#svc\.recordActivity\(r\.Context\(\),\ claims\.PlayerID\)#// activity observation removed#3' server/internal/auth/middleware.go
```

Förväntat och faktiskt assertion-rött: `TestAuthenticatedActivityLifecycle`. [Rålogg](mutations/middleware-optional.log).

### db-gate

```sh
sed -z -E -i 's#\(last_seen_at\ IS\ NULL\ OR\ last_seen_at\ <=\ \$2::timestamptz\ \-\ interval\ '"'"'5\ minutes'"'"'\)#true#1' server/internal/auth/activity.go
```

Förväntat och faktiskt assertion-rött: `TestActivityConcurrentRequestsWriteOnce`. [Rålogg](mutations/db-gate.log).

### cache-gate

```sh
sed -z -E -i 's#ok\ \&\&\ now\.Before\(next\)#ok \&\& false \&\& now.Before(next)#1' server/internal/auth/activity.go
```

Förväntat och faktiskt assertion-rött: `TestAuthenticatedActivityLifecycle`. [Rålogg](mutations/cache-gate.log).

### failure-retry

```sh
sed -z -E -i 's#delete\(s\.activityNext,\ playerID\)#// failed observation cache kept#1' server/internal/auth/activity.go
```

Förväntat och faktiskt assertion-rött: `TestActivityFailureDoesNotRejectAuthentication`. [Rålogg](mutations/failure-retry.log).

### five-minute-boundary

```sh
sed -z -E -i 's#interval\ '"'"'5\ minutes'"'"'#interval '"'"'4 minutes'"'"'#1' server/internal/auth/activity.go
```

Förväntat och faktiskt assertion-rött: `TestActivityFiveMinuteGateAndCacheExpiry`. [Rålogg](mutations/five-minute-boundary.log).

### null-observation

```sh
sed -z -E -i 's#last_seen_at\ IS\ NULL\ OR\ ##1' server/internal/auth/activity.go
```

Förväntat och faktiskt assertion-rött: `TestAuthenticatedActivityLifecycle`. [Rålogg](mutations/null-observation.log).

### migration-no-backfill

```sh
sed -z -E -i 's#TIMESTAMPTZ;#TIMESTAMPTZ DEFAULT now();#1' server/db/migrations/216_player_activity.up.sql
```

Förväntat och faktiskt assertion-rött: `TestAuthenticatedActivityLifecycle`. [Rålogg](mutations/migration-no-backfill.log).
