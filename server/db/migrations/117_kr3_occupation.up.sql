-- Migration 117: KR3 erövringens efterspel (megaron_plan_erovring.md).
--
-- A won siege no longer annexes directly (that was the old one-shot
-- applyAttackerWins, now unreachable — see combat/battle.go's header
-- comment). Instead a winning attacker holds the city as an OCCUPIER: the
-- settlement stays owned by the defeated Wanax (owner_id unchanged) while
-- state='occupied' and occupant_id names the army holding it. A counter
-- (occupied_since_tick) matures into an annex offer after
-- occupationTicksToAnnex (combat/occupation.go) IF the occupation goes
-- unchallenged; any attack against the occupied city resets the counter
-- (the contestable async window — a defender/ally has time to relieve it).
-- annex_ready_notified guards the one-time "you may annex now" notice from
-- re-firing every time the self-rescheduling ScheduledOccupationCheck wakes
-- up, and is reset to false whenever the counter itself resets.
--
-- recolonizable_after_tick is the sack-and-burn karens (S5): a razed
-- settlement blocks colonize-in-place FOREVER today (march_start.go's
-- existing-settlement check has no state filter at all) — this column lets
-- that check make an exception once the karens has elapsed, replacing the
-- permanent block with a temporary one for the burn path specifically. NULL
-- means "no karens running" (every settlement not currently mid-karens,
-- including the old permanent-collapse paths this migration does not touch).
ALTER TABLE settlements
    ADD COLUMN occupant_id              UUID REFERENCES players(id),
    ADD COLUMN occupied_since_tick      INT,
    ADD COLUMN annex_ready_notified     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN recolonizable_after_tick INT;

CREATE INDEX idx_settlements_occupied ON settlements (state) WHERE state = 'occupied';
