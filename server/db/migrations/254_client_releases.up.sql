-- Released client binaries (keryx): the newest row per client is what the server
-- announces in the X-Keryx-Latest response header. Written only by
-- tools/announce-keryx-release.sh; the same commit is never recorded twice.
CREATE TABLE client_releases (
    client      TEXT NOT NULL,
    commit      TEXT NOT NULL,
    released_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (client, commit)
);
