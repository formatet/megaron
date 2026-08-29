-- Down återställer bara STRUKTUREN, inte datan. Original var NOT NULL UNIQUE,
-- men det kan inte satisfieras för befintliga rader utan värden — så nullable.
ALTER TABLE players ADD COLUMN email TEXT;
