# O — Notifications + Gossip i en lista

Rumours (`/gossip`) are merged into the Notifications list, newest first, each marked
🗣 *Rumour* (a rumour is not an event: blurred, maybe old). Critical subsistence warnings
still float to the top; a kind drill-down ("show reports") stays events-only; a failed
rumour fetch never hides events. The Gossip button and drawer are gone; "Known Wanaxes"
is covered by Diplomacy → Known. Codex articles `gossip`, `screen`, `coming-back` updated.
No server/API change. `gossip.js` keeps its exports (`loadGossipDrawer`, `renderWanaxesHTML`)
as unused code — not deleted, to keep this slice surgical.

Evidence: 440 JS tests (new `O:` test); mutation (no sort) -> red; `list-*.png` rendered in real
Chromium with stubbed HTTP (`python3 tools/en_lista_shot.py OUT`) — isolated fixture, not a real-game proof.
