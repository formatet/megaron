# Isolated Agora acceptance rig

This runs the official signed Debian package, pinned to Continuwuity **26.9.1**.
It does not read production configuration or contact production Agora. Use a unique
Compose project and fixture directory; do not reuse the game's acceptance project.

```sh
python3 tools/agora_acceptance/rig.py init --dir /tmp/megaron-agora-private --port 18099
AGORA_RIG_DIR=/tmp/megaron-agora-private AGORA_RIG_PORT=18099 docker compose -p megaron-agora-proof -f tools/agora_acceptance/compose.yaml up -d --build
AGORA_RIG_DIR=/tmp/megaron-agora-private AGORA_RIG_PORT=18099 docker compose -p megaron-agora-proof -f tools/agora_acceptance/compose.yaml exec matrix /usr/bin/conduwuit --version
python3 tools/agora_acceptance/rig.py bootstrap --dir /tmp/megaron-agora-private --port 18099
python3 tools/agora_acceptance/rig.py verify --dir /tmp/megaron-agora-private --port 18099
```

Wait until the server is ready before bootstrap. Fixture directory is mode 0700;
config and secret.json are mode 0600. Read credentials programmatically; never
print them, put them in shell arguments, or copy them into git. `secret.json`
contains `base_url`, `room`, and `temenos_token` for the game's isolated acceptance.
The emergency bot password bootstraps a real `@temenos:agora.test` admin account.
Container logging is disabled and the server log filter is `off`.

Admin-room Matrix events themselves contain passwords in both requests and replies.
The no-password-in-events requirement applies to the game's events/notifications;
this transport does not promise absence from Matrix's private admin-room history.

Verified 26.9.1 (`046074bbc`) syntax:

* `!admin users create-user LOCALPART PASSWORD`: `create-user` is a working alias
  of `create`. Success body ends in `Created user @LOCALPART:agora.test with password`
  followed by a backtick-quoted password. A log table may precede the success line.
* Active and deactivated collisions both return `Command failed with error:` with
  a fenced `Username is not available.` message. Do not adopt based on this alone.
* `!admin users reset-password LOCALPART PASSWORD`: success begins
  `Successfully reset the password for user @LOCALPART:agora.test:`. Optional
  `--logout` also invalidates existing sessions. Old password login is M_FORBIDDEN;
  new password login succeeds.
* `!admin users deactivate @LOCALPART:agora.test`: success is
  `User @LOCALPART:agora.test has been deactivated`.
* Final replies use `m.notice` and
  `m.relates_to.m.in_reply_to.event_id` matching the request event ID. Ignore
  intermediate uncorrelated messages, even when sent by `@conduit:agora.test`.
* Login as the created user, then PUT `/profile/{userId}/displayname` with
  `{"displayname":"Fíxture Wanax"}` returns `{}`. GET confirms the original name.

Repeated PUT `/rooms/{room}/send/m.room.message/{stableTransactionId}` with the same
admin token returns the original event ID even if the new password payload differs.
Ownership can therefore be proven from the original correlated success reply.
`/rooms/{room}/messages?dir=b&limit=100` returns `start`, `end`, `chunk`; paginate
backward with `from=end`. `/rooms/{room}/context/{eventId}?limit=20` returns `event`,
`events_before`, `events_after`, `start`, `end`, `state`; the reply was recovered in
`events_after`. Historical event bodies must never enter diagnostic output.

This script proves Matrix functional password login. Element browser acceptance is
separate and still required before calling the whole player scenario complete.

All three commands support `--` before positional arguments; verified with a
localpart starting with `-`. Use the delimiter to avoid interpreting a name or
password as a flag. The rig also prefixes generated passwords with an ASCII letter.


## Separate game and Element acceptance

This project does not use the main compose or the old `megaron-acc` project. It
owns PostgreSQL on localhost 18432 and Redis on localhost 18379, with separate
volumes. The local Go game process uses the existing all-interface `:18097` bind;
requests use localhost. Stop that temporary process after acceptance unless the
reviewer needs its picture-review URL. No production listen behavior is changed.

```sh
docker compose -p megaron-agora-game -f tools/agora_acceptance/game-compose.yaml up -d
python3 tools/agora_acceptance/game.py prepare --dir /tmp/agora-game-private --matrix-fixture /tmp/agora-rig-clean/secret.json
# Build/start only after the backend slice is ready:
python3 tools/agora_acceptance/game.py build --dir /tmp/agora-game-private
python3 tools/agora_acceptance/game.py run --dir /tmp/agora-game-private
```

`game-env.json` contains only explicit isolated settings and fresh fixture
credentials, mode 0600. It never copies or sources the developer .env. Credentials
are loaded directly into the game subprocess, never passed in command arguments.

```sh
python3 tools/agora_acceptance/element.py prepare --dir /tmp/agora-element-private --matrix-port 18100
python3 tools/agora_acceptance/element.py serve --dir /tmp/agora-element-private --port 18098
```

Element Web 1.12.30 is downloaded from the official GitHub release and checked
against release asset SHA256
`41ecae1e7af5d09baf2c1a7646cebad51ced6a44257d2096b9618bff3fee625d`.
The quiet HTTP server binds only localhost and serves `.mjs` as JavaScript. It
pins the local Matrix homeserver and disables custom URLs and guest login.

Browser proof uses Python Playwright and a private fixture JSON with `element_url`,
`matrix_user`, `matrix_password`, and optional `matrix_old_password`. It clicks
through Element's optional device-verification skip, sends no Matrix messages,
and saves no password-bearing screenshots, traces or browser sessions.

```sh
python3 tools/agora_acceptance/element_browser.py --fixture /tmp/agora-element-private/browser-fixture.json --old-password
```


Actual player scenarios use the running isolated game, Matrix and Element above.
Build a Keryx binary from this checkout into the private game directory. The game
helper records its own PID with mode 0600 and defaults its Go cache to the fixture
directory. Store game stdout/stderr only in a private mode-0600 game.log.

```sh
# Run each scenario sequentially; all credentials stay in mode-0600 fixtures.
python3 tools/agora_acceptance/player.py --dir /tmp/agora-player-proof-final --matrix-fixture /tmp/agora-rig-clean/secret.json --scenario basic --keryx /tmp/agora-game-private/keryx --game-log /tmp/agora-game-private/game.log
python3 tools/agora_acceptance/player.py --dir /tmp/agora-player-proof-final --matrix-fixture /tmp/agora-rig-clean/secret.json --scenario collision --game-log /tmp/agora-game-private/game.log
python3 tools/agora_acceptance/player.py --dir /tmp/agora-player-proof-final --matrix-fixture /tmp/agora-rig-clean/secret.json --scenario outage --game-log /tmp/agora-game-private/game.log
python3 tools/agora_acceptance/player.py --dir /tmp/agora-player-proof-final --matrix-fixture /tmp/agora-rig-clean/secret.json --scenario crash --second-pass --game-log /tmp/agora-game-private/game.log
```

The crash scenario creates a temporary trigger in its own verified Compose DB,
limited to the newly registered player UUID. It blocks the final ready update
after real remote provisioning, verifies the private game PID's executable path,
terminates only that process, removes the trigger and restarts the same game.
Cleanup also removes the trigger and restores the process after a failure.
It requires one original Matrix CREATE event and correlated success, one ready
notification after another pass, and nonempty real provisioning secret canaries
absent from the whole game DB and log. Browser and rotations are proved by basic;
crash reports those checks as skipped. No production hooks are added.

When an agent tool starts a fresh PID namespace for each call, its PID file cannot
safely control the process from another call. Run the narrowly scoped host helper
through the tool's host-execution approval instead. It verifies the private
configuration, executable, argv, working directory, owner and isolated endpoints;
it controls only this game process. It never prints process environments.

```sh
python3 tools/agora_acceptance/host.py status
python3 tools/agora_acceptance/host.py serve
# In another host tool call, while the foreground game session remains alive:
python3 tools/agora_acceptance/host.py scenario --scenario basic
python3 tools/agora_acceptance/host.py scenario --scenario collision
python3 tools/agora_acceptance/host.py scenario --scenario outage
python3 tools/agora_acceptance/host.py scenario --scenario crash
```

The host helper retains the existing game bind on `:18097`; requests and all
upstream services use the explicitly isolated localhost endpoints above.
