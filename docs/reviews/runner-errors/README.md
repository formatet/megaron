# Runner and passage refusals — inventory (A3, 2026-10-21)

Plan: `megaron_plan_budvagar_och_murtak_20261021.md` §A3. Pattern: **what · why · what you can do**, by name,
never a uuid; durations in game days (the server cannot know the reader's clock, so "when" is "in N days").

## Rewritten

| Where | Before | After (shape) | Why it mattered |
|---|---|---|---|
| `api/handlers/messenger_passage.go` Arrange, ship not free | "the ship must be docked in its own port, free of any mission, to be sent on passage" | per status: *Silent Galley is still being built — 2 days to go. Arrange passage once it is launched.* · repairs · fitting out · laid up · freighting · away on a mission (+ days to destination) | Live: Hephaistos (round 161–163) got 422 on a ship under construction with no hint it would be ready soon |
| same, ship in another own port | "the ship must be docked at X — the runner is waiting there" | *Galley lies in Kydonia, but your runner is waiting in Phalasarna. Choose a ship docked in Phalasarna, or sail Galley there first.* | named only one of the two ports |
| same, not your ship | "not your ship" | *X belongs to another Wanax — you can only send your own ships. Your runner boards any ship that sails its way by itself.* | A1 made foreign ships carriers; the refusal now says so |
| same, war galley | "a war galley cannot carry a runner — …" | *X is a war galley and carries no passengers. Send a Galley or an Emporos instead.* | names the ship and the in-game type names |
| same, runner not waiting | "this runner is not waiting for passage" | aboard / ship lost (sealed) / on the road or arrived | said nothing about where it was |
| same + call-back, not your runner | "not your runner" | *That runner is not yours — only the Wanax who sent it can …* | |
| call-back, foreign port (`ErrCallBackNotOwnPort`) | "runner is waiting in a foreign port — call-back only works …" | *…waiting in another Wanax's port on its way home — only a runner still in your own port can be called back. Arrange passage to fetch it, or let it wait: any ship sailing its way takes it aboard* | the live Dunios case: now has a way out |
| call-back, not waiting | "this runner is not currently waiting for passage" | *That runner is not waiting in port — only a runner still waiting for a ship can be called back.* | |
| send, no port (`ErrNoPort`) | "no port of yours to take ship from — …" | *your runner cannot cross the sea: you hold no coastal or harboured city to take ship from. Send it from a coastal city, or build a Harbour* | gives the two actions |
| PassageStalled dispatch | web and keryx each wrote their own line ("No eligible ship of yours is in port yet — build or send one.") | server field `explanation` (`messenger.stalledExplanation`), printed by both: where, why (no ship sailed toward X), that ANY ship takes it, what you can do; call-back only offered in an own port | one source; the old line contradicted A1 |
| keryx outbox waiting line (`cmd/keryx/output.go`) | "waiting … for a ship of yours to sail that way" | "… for any ship to sail that way" | A1 |

## Left as they are (already what · why · do, or not player-reachable)

- `api/handlers/messenger.go` Send/Reply/trade: scouted range, duplicate pending offer, ship capacity, no free ship, silver/cult not tradeable, people on the move — already name the reason and the action.
- `invalid world/messenger/settlement ID`, `invalid JSON`, `ship_id is required`, `… not in this world` — client/agent bugs, not player situations.
- `combat.StartMarch` rejections passed through by Arrange (`rej.Reason`) — the march verb's own texts, outside this slice.
- `OrderFailed` "your runner's ship was lost — sealed and safe, back awaiting passage in port shortly" — already says what and next.
- Diplomacy outbox hint "no ship of yours is free there yet" (`web/static/js/megaron/ui/drawers/diplomacy.js`) — true for *Arrange passage* (own ships only); revisited in A4's Runners tab.

## Proof

- `server/api/handlers/messenger_passage_texts_test.go` — red on old code (old texts printed in the failure), green now; asserts ship name, "still being built", days, both port names, the "by itself" pointer, no uuid.
- `server/internal/messenger/stalled_explanation_test.go` — foreign port never offers call-back; own port does.
- `web/static/js/megaron/ui/dispatch_window.test.mjs` — the dispatch prints the server explanation, not the old contradicting line.
- TEXT gate: judged by the AI players in their next round (`/home/tk/.local/state/megaron-luna/<player>/last-summary.md`).
