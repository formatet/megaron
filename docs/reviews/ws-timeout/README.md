# WS timeout — proof package

## SAMMANFATTNING

A healthy WebSocket could remain open, then produce a false HTTP 504 and
`response.WriteHeader on hijacked connection` when it disconnected. The root
HTTP middleware gave every request a 30-second context deadline. chi's Timeout
writes 504 in its deferred cleanup after the handler returns if that deadline
expired, including when the response connection has already been hijacked.

Only `/ws/` paths now bypass that Timeout middleware. Every other path retains
exactly 30 seconds. The middleware order, HTTP server configuration, WebSocket
handler and hub are unchanged. Production commit: `07f5ae93a9cfe711d82cd9cfd4197e3dd2a3a0eb`;
base: `713fe035`. Claude reviews/integrates; not deployed by Codex.

Final Firefox 146.0.1 proof: connection open **65,021ms**, two heartbeats,
clean close code1000, zero WS errors, no 504/hijacked-write in the complete
observation-window server log. Full Go **25 test packages green**, fresh migration171.
Build log whitespace/carriage-return progress normalized for version control.

Measured rig: see [provenance.txt](provenance.txt) and [metadata.json](metadata.json).
The latter records source hashes and identical local/container binary hashes.

## SLICE-KONTRAKT

This proves the transport supporting the geography → shortage → bronze → elite
chain, at the web client's persistent push connection. No game verb, gameplay,
text or visual change. `/ws/` bypasses HTTP request timeout; all other routes
retain precisely 30 seconds at the same middleware position. Compression and
Hijacker forwarding remain intact. Long-lived WS must close without a false
504 or a write to a hijacked response.

## BASLINJE

C's real acceptance run on `ad2dc277` recorded the false 504/hijacked-write on
browser context closure: `../markraster-spelet/server-transport.log`.
The new `TestRequestTimeoutMiddleware` through the actual `installMiddleware`
is red on the unmodified base for both `/ws/world` and `/ws/`; see
[baseline.log](baseline.log). API, static, health, root and lookalike prefixes
already have the correct 30-second deadlines.

## EXPERIMENT

| Hypothesis | Change/check | Result | Decision |
|---|---|---|---|
| Root request timeout writes after WS disconnect | Read pinned chi `middleware/timeout.go` and actual middleware/handler | Deferred `DeadlineExceeded` → `WriteHeader(504)` after hub returns | Bypass only `/ws/` at same position |
| WS needs request deadline | Trace `WSHandler.Connect` and `Hub.Register` | Context only supplies player ID; Register takes no context; pumps use connection I/O | No handler/hub change |
| HTTP WriteTimeout 15s kills WS | Trace gorilla Upgrade, handler and Go HTTP server | Upgrade and handler clear raw deadlines; HTTP server leaves hijacked connections to caller | Keep server timeouts |
| Guard catches lost exception | Physically disable prefix branch, run actual-chain test | Named red for both WS cases; byte-identical restore | Keep test; [mutation.log](mutation.log) |
| Real connection survives and closes cleanly | Actual Firefox upgrade against rig, 65s hold, normal close | See [ws-rigg.json](ws-rigg.json), [ws-server.log](ws-server.log) | No 504/hijacked-write |

The unchanged chi Logger records `000 0B` for the upgraded response because
gorilla writes the handshake directly on the hijacked connection. The browser
opening event, heartbeat frames and close code prove the actual upgrade; the
logger row proves the handler returned without recording a 504.

Hub transport deadlines remain read 60s, refreshed on protocol pong; writes
10s per frame, with ping plus app heartbeat every 25s. Firefox auto-pongs. Gorilla
clears HTTP deadlines in Upgrade; the handler also calls NetConn.SetDeadline(zero).
Go's HTTP serving loop returns after a hijacked handler and does not finish/reuse
that connection. WS lifetime therefore belongs to the existing hub pumps.

## GRINDAR

| Gate | Evidence |
|---|---|
| Code | Targeted real-chain deadline/compression tests, physical mutation and restore; [targeted.log](targeted.log). Fresh PostgreSQL 16 migration171 full Go suite in [full-go.log](full-go.log); cmd/server + notify vet clean |
| Visual | Not applicable: no rendering/UI change |
| Semantic | No deadline introduced on WS; exactly 30s on non-WS, including `/ws` and `/ws-other`; no auth/FOW changes |
| User | Real Firefox WS route and real hub, >60s, two app heartbeats, graceful close; anonymous subscription supported by existing handler; no injected state or DB writes |
| Drift | Health identifies production core `07f5ae9`, migration171/171; binary hash matches; rig proof only, not deployed |

The first observer run completed its 65s connection and clean close, but an overly
broad log assertion matched `504` inside an unrelated tick timestamp. The observer
now matches the HTTP status field; the entire 65s scenario was rerun. No production
change was made for this observer false positive.

## METODISK LÄRDOM

Trace middleware cleanup as well as handler execution: a successfully hijacked
response may still be written by an outer deferred middleware on disconnect.

## KÄNDA AVGRÄNSNINGAR

This is a transport test, not another four-surface gameplay acceptance run.
The existing acceptance world is retained; no reseed/player/resource mutation.
The rig has the same Alpine/registry DNS restriction found in C: local CGO-disabled
Go build of the exact core commit is layered onto the current cached runtime
through a temporary Compose build override, then started with `acceptance.sh up`.
This does not prove a clean network-dependent Docker build. Overrides and build
context live under `/tmp/markraster-acc-build`, not in production files.

Reproduce the observer with `python3 tools/ws_timeout_rigg.py` (Playwright Firefox)
after `tools/acceptance.sh up` and confirming `/healthz`. Run
`tools/gotest.sh ./cmd/server -run 'TestRequestTimeoutMiddleware|TestCompress'`
for the focused contract. To reproduce the mutation, change the real branch to
`if false && strings.HasPrefix(r.URL.Path, "/ws/")`, run
`go test ./cmd/server -run TestRequestTimeoutMiddleware -count=1` from `server/`,
then restore the original file byte for byte (SHA256 in metadata). The report commit intentionally differs from the core
commit reported by the already-running rig; docs/tool add no server change.
