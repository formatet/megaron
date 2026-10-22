-- The application completes the workforce transfer and production recomputation
-- atomically before starting any worker or serving HTTP. No additive staffing.
CREATE TABLE temple_staffing_transition (
 id boolean PRIMARY KEY DEFAULT true CHECK(id),
 completed boolean NOT NULL DEFAULT false
);
INSERT INTO temple_staffing_transition(id,completed) VALUES(true,false);
