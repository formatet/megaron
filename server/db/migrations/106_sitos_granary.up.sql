-- Migration 106: Sitos-fonden blir ett MAGASIN.
--
-- Migration 072 gave every settlement a silver pool that was supposed to
-- stabilise the price of subsistence goods. Measured over a whole playthrough
-- it moved +1710 grain net — no effect — while binding ~340 000 silver against
-- a world with 106 678 liquid, and its head tax produced 2132 desertions, all
-- silver_shortage. Three roots, all code-read 2026-08-02:
--
--   1. The trigger measured the wrong quantity. RefPrice was a moving average
--      of the SAME LocalPrice it was compared against — a momentum detector,
--      silent at every equilibrium including a stable famine.
--   2. The price band was in absolute silver (0.5–3.0) while the formula's
--      range is 0.5×base…3×base, i.e. 1.5–9.0 for grain: the floor could never
--      bind, the ceiling always did.
--   3. It stored nothing. `stabilizeGood` DESTROYED grain on a buy and CREATED
--      it on a sell. The name σῖτος promises a grain-watcher; the code was a
--      price oracle with a silver balance sheet.
--
-- Timothy's decisions 2026-08-02 (megaron_valtabell §Besvarat 2026-08-02):
--   B1 the trigger is COVERAGE IN DAYS, not price
--   B2 E1 is struck — the granary may help in famine; the only limit is that
--      it can be empty
--   B3 silver enters the game ONLY via starting silver and mines
--   B4 it fills with a TITHE of the surplus
--   B6 it holds the whole food basket, grain AND fish
--
-- Full plan: temenos_sitos_magasin_plan.md.

CREATE TABLE settlement_granary (
    settlement_id uuid   NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    good_key      text   NOT NULL REFERENCES goods(key),
    amount        float8 NOT NULL DEFAULT 0 CHECK (amount >= 0),
    PRIMARY KEY (settlement_id, good_key)
);

COMMENT ON TABLE settlement_granary IS
    'Sitos granary: food the city has set aside. Strictly conserved — food moves '
    'city <-> granary and is never created or destroyed. Holds the food basket '
    '(grain and fish, B6). Never silver (B3).';

-- Conserve the silver already bound in the funds instead of deleting it with
-- the column. Destroying ~340 000 silver would be a deflation event of exactly
-- the kind the silver plan exists to prevent, and SilverAudit's net_delta would
-- take an artificial cliff that no later reader could distinguish from a bug.
-- The cap is raised by the same amount so the fund's balance cannot be clipped
-- on the way in: the city gets back what it was already holding on its behalf.
--
-- Settlements without a silver row cannot receive it. There are none in
-- practice (genesis always writes one), but the WHERE is explicit rather than
-- silent about it.
UPDATE settlement_goods sg
SET amount = sg.amount + GREATEST(0, s.sitos_fund_silver),
    cap    = sg.cap    + GREATEST(0, s.sitos_fund_silver)
FROM settlements s
WHERE sg.settlement_id = s.id
  AND sg.good_key = 'silver'
  AND s.sitos_fund_silver > 0;

ALTER TABLE settlements DROP COLUMN sitos_fund_silver;
