-- Budbärare från Nomadic Host: före grundningen finns ingen settlement att sända
-- ifrån, men designen kräver att hostet kan nå sina spearmen, ta första kontakt
-- och skicka vanliga meddelanden (temenos_nomadic_host_plan.md §Budbärare).
--
-- Speglar mig 035, som gjorde exakt detta åt DESTINATIONEN (destination_id
-- nullable + dest_q/dest_r för fält-mål). Här görs samma sak åt origin.
--
-- origin_q/origin_r VALDES framför en join mot units.q/r (temenos_nomadic_host_
-- fas4_plan.md lämnade valet öppet) därför att avsändarpunkten är ett fysiskt
-- faktum fryst vid avsändningen: hostet VANDRAR under budbärarens resa och
-- upplöses helt vid grundningen (q/r → NULL) — en join hade ritat fel färdväg på
-- kartan och blivit NULL mitt i en pågående leverans. Settlement-budbärare rör
-- inte kolumnerna (städer flyttar sig inte; deras koordinater joinas som förut).

ALTER TABLE messengers
    ADD COLUMN origin_unit_id UUID REFERENCES units(id),
    ADD COLUMN origin_q INT,
    ADD COLUMN origin_r INT;

ALTER TABLE messengers ALTER COLUMN origin_id DROP NOT NULL;

-- Exakt EN avsändare: settlement eller enhet, aldrig båda, aldrig ingen.
ALTER TABLE messengers ADD CONSTRAINT messengers_exactly_one_origin
    CHECK ((origin_id IS NULL) <> (origin_unit_id IS NULL));

-- En enhets-avsänd budbärare måste bära sin frysta avsändarpunkt.
ALTER TABLE messengers ADD CONSTRAINT messengers_unit_origin_has_coords
    CHECK (origin_unit_id IS NULL OR (origin_q IS NOT NULL AND origin_r IS NOT NULL));
