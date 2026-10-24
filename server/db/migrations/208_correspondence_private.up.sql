-- One private aggregate per person and already-existing shared case. No city FK:
-- acquiring or deleting a city neither grants nor removes this private memory.
CREATE TABLE correspondence_private (
 case_id uuid NOT NULL REFERENCES correspondence_cases(id) ON DELETE CASCADE,
 player_id uuid NOT NULL REFERENCES players(id),
 draft text NOT NULL DEFAULT '' CHECK(octet_length(draft)<=1000),
 note text NOT NULL DEFAULT '' CHECK(octet_length(note)<=10000),
 version bigint NOT NULL CHECK(version>0),
 PRIMARY KEY(case_id,player_id)
);
