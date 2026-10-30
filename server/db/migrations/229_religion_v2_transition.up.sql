-- A single startup transaction announces the new rule and installs its shield.
CREATE TABLE religion_v2_transition (
 id boolean PRIMARY KEY DEFAULT true CHECK(id),
 completed boolean NOT NULL DEFAULT false
);
INSERT INTO religion_v2_transition(id) VALUES(true);
