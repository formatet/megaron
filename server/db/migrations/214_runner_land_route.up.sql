-- NULL retains the frozen legacy interpretation for already-dispatched runners.
ALTER TABLE messengers ADD COLUMN land_route jsonb;
