-- Migration 150: the abstract boat goes away (megaron_plan_ordna_passage.md,
-- slice 3b-4).
--
-- With the RESERVE removed (R1), a messenger stuck 'awaiting_passage' with no
-- carrier no longer resolves itself — it is a player decision, made through a
-- dispatch (Timothy 2026-09-27, megaron_styrande_beslut.md "Havet är fysiskt":
-- "det är en helt ny mekanik ... men det ska ske enligt en dispatch — spelaren
-- får avgöra. Ingen automatisk reserv, ingen tidsgräns.").
--
-- passage_stalled_notified_tick (R5): the tick the current PassageStalled
-- dispatch was sent, or NULL if none has fired yet for this waiting spell.
-- Reset to NULL every time a messenger (re)enters 'awaiting_passage' after a
-- carrier loss (PassageScanHandler.promoteSealed) so a NEW stall notice can
-- fire for the new spell — "en gång per väntperiod ... igen efter en ny
-- försegling, men inte varje skanning".
--
-- withdrawn (R5's "kalla tillbaka"): true once a Wanax has called an outbound
-- runner back from their own port before it ever crossed. The runner's return
-- leg (status='returning') runs exactly like any other, but never delivered —
-- ReturnHandler's home-arrival notice reads this flag to say "came home
-- undelivered" / "order withdrawn" instead of the ordinary no-reply text.
ALTER TABLE messengers
    ADD COLUMN passage_stalled_notified_tick INT,
    ADD COLUMN withdrawn BOOLEAN NOT NULL DEFAULT FALSE;
