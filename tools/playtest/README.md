# Luna playtest fleet

Six independent Claude Haiku players (GPT-6 Luna until 2026-10-21) in the existing 40×40 test world, driven
by ordinary Keryx. The controller has no database access. It reads the public
world clock, runs at most two players concurrently, waits five ticks after each
finished round, and caps a round at 24 CLI calls and 480 seconds. Three repeated
failures back off for 30 ticks. World changes never trigger automatic rejoining.

`luna_prompt.md` supplies the shared exploration/contact/trade goal. Each model
receives only its own prior state card, and operates only its own `./kx` wrapper.
The CLI retains all ordinary player validations, fog, physical orders and travel.
No model sees administrative map information or another player's private state.
CLI refusals are recorded as refusals, not silently fixed by the controller.

Each round runs `claude -p --model claude-haiku-5-5 --permission-mode dontAsk` (Timothy
2026-10-21: Haiku plays so Codex quota goes to development). API keys and player
tokens are stripped from the child environment. At most two model rounds run
concurrently; the service does not require this chat to stay open.

## Runtime paths

- Code/prompt: `~/.local/share/megaron-luna/` (copied from this directory; this
  directory is the source — change here, then copy).
- Own workspace/config/audit/notes: `~/.local/state/megaron-luna/<player>/`.
- Credentials: `config.json`, mode 600; never commit credentials or runtime logs.
- `~/.config/megaron-ai/<name>.json` links to the authoritative live profile.
  Pre-link copies are preserved privately in the playtest archive.
- `luna_command.py` renews access/refresh tokens before access expiry and retries
  one genuine 401 after renewal. Failed renewal preserves the original config.
- Old local `~/.config/poleia/` files and dirty isladan sources are untouched.

## Control

```sh
python3 ~/.local/share/megaron-luna/luna_fleet.py status
systemctl --user status megaron-luna-fleet.service
systemctl --user stop megaron-luna-fleet.service
systemctl --user start megaron-luna-fleet.service
```

The installed user service is enabled; this machine has `Linger=yes`. The machine,
network, Codex sign-in/quota, and game server must remain available. Do not start
the retired 35B agents on the same accounts. Stop the fleet before changing code
or profiles so no game orders race. A single controller holds `fleet.lock`.

## Reinstall/update

Stop the user service. Copy `luna_fleet.py`, `luna_command.py`, and
`luna_prompt.md` here to `~/.local/share/megaron-luna/`. Run
`python3 ~/.local/share/megaron-luna/luna_fleet.py setup` to regenerate the own
workspace wrappers. Existing credentials and notes are preserved. Install the
unit here to `~/.config/systemd/user/`, adapt PATH if Codex is installed elsewhere,
then `systemctl --user daemon-reload` and enable/start the unit. `setup` requires
the six already-authorized player configs; it never registers new accounts.

Validate with `python3 -m unittest discover -s tools/playtest -p test_luna.py -v`.
Actual first-round and clock-change evidence: `docs/reviews/femspelare40/SIX-LUNA.md`.
