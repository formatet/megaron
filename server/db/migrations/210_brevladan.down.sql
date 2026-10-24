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

CREATE TABLE correspondence_citations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 case_id uuid NOT NULL REFERENCES correspondence_cases(id) ON DELETE CASCADE,
 player_id uuid NOT NULL REFERENCES players(id),
 letter_id uuid NOT NULL REFERENCES correspondence_letters(id) ON DELETE CASCADE,
 source text NOT NULL CHECK(source IN ('message','reply')),
 quote text NOT NULL CHECK(octet_length(quote) BETWEEN 1 AND 1000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 created_epoch timestamptz NOT NULL DEFAULT pg_postmaster_start_time(),
 UNIQUE(player_id,letter_id,source,quote)
);
CREATE INDEX correspondence_citations_person ON correspondence_citations(player_id,case_id,created_at DESC,id DESC);
