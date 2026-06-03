-- Recall-via-budbärare: order till egna enheter (armé/utpost) bärs av en fysisk budbärare
-- som reser ut till enheten i fält. Sådana budbärare har ingen bosättnings-destination —
-- de siktar på en provins-koordinat. Utöka messengers att bära detta.

ALTER TABLE messengers
    ADD COLUMN kind   TEXT NOT NULL DEFAULT 'diplomatic',  -- 'diplomatic' | 'recall'
    ADD COLUMN dest_q INT,                                 -- fält-mål (provins) när destination_id är NULL
    ADD COLUMN dest_r INT;

-- En recall-budbärare har ingen mottagande bosättning.
ALTER TABLE messengers ALTER COLUMN destination_id DROP NOT NULL;
