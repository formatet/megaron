-- Reading a letter is independent of where its physical messenger stands.
ALTER TABLE messengers ADD COLUMN read_at TIMESTAMPTZ;
