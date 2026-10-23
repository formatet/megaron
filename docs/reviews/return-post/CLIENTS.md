# Return-post client proof

Queue shows the saved `reply_deadline_tick` through the shared world-time formatter, with `reply_before` as its ISO fallback. The server's `reply_window_closed` removes Reply; clients never infer this cutoff from wall time. An old delivered letter without a saved deadline remains answerable when permitted by the server and says the deadline is unavailable. An already answered letter says **Reply dispatched**, including after its old window has closed.

Legacy Correspondence and Keryx inbox/history suppress the closed reply affordance. A cutoff refusal arriving after a form was opened appears beside its retained draft rather than disappearing silently. The Codex explains the same bearer, arrival/following-day window, local departure deadline, prior longer stays and independent trade deadline. No new send route or origin-discovery rule is added.

The chosen server linearization point is its world-tick read **after locking the bearer and recipient settlement**, inside the reply transaction. This defines whether the reply is accepted; it is not a promise to serialize all world progress until commit. Late delivery starts its new plain-letter stay from the receipt transaction's tick read. No new world-row lock order was introduced.

## Evidence

- Full JavaScript: 752 passed, 0 failed/skipped, `client-full-js.log`; vet passed, `client-vet.log`.
- After the final two-line legacy refusal display change: 20 focused Queue/Correspondence tests passed, `client-js-final-targeted.log`; no further broad suite repeated.
- Four physical consumer source mutants fail named tests and restore exact original bytes, then pass: closed Queue reply, closed legacy thread reply, closed CLI reply and silently ignored cutoff refusal. Reproduce with `python3 tools/return_post_client_mutations.py`; SHA records/logs are adjacent.
- Fresh owned runtime `09b57de+returnpost`, migration206: `run1/proof.json`. Normal register/join/found; two explicitly seeded **outbound** messengers and scheduled Arrival events (due100 and late90). The real Arrival worker receives both at100 and saves stay deadlines102. They are not hand-marked delivered.
- Actual Keryx displays the local saved deadline. Native Firefox sends a reply through the exact delivered messenger ID; DB retains two messengers, one returning with the requested words and exactly one MessengerReturn event. No new-message POST occurs. Controlled tick102 then removes Reply in Queue and legacy Correspondence, and Keryx history carries no dead reply command. This fixture does not change incoming-contact/FOW policy.
- Desktop1280/mobile390 pictures inspected at native1:1, no horizontal overflow or page errors. `index.html` links the four final pictures.

The first successful runtime's final picture used a stale browser100 anchor after SQL advanced the world to102 without a WebSocket event. Saved as `pre-anchor-refresh-run1`; the final runtime reloads actual world bootstrap after that controlled SQL advance, so the closed-state local time is projected from102. This was a proof-rig correction, not a production change.

The runtime predates only the final legacy refusal **error-display** addition; that path has its targeted actual-handler test and source mutation above. Successful reply, state/deadline rendering and server semantics are the runtime-proven source.
