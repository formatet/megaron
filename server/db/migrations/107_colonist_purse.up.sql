-- Migration 107: kolonistens börs — silver bärs, myntas inte.
--
-- Every founding minted a colony's starting silver out of nothing:
-- GenesisSilverLiquid(pop) with pop = 1500 + the colonising unit's size, so a
-- ~2000-pop colony printed ~10 500 liquid silver into a world whose entire
-- liquid supply measured 106 678 in playtest. Expansion was a printing press,
-- which hollows out the canon principle that silver running dry IS the collapse
-- mechanic: you could always found your way out of insolvency.
--
-- B3 (Timothy 2026-08-02): silver enters the game ONLY via starting silver and
-- mines. World genesis stays a sanctioned faucet — the capital and the nomadic
-- host still mint. Every founding after that MOVES silver: the colonists carry a
-- purse from the city that sent them.
--
-- carried_silver is that purse. It is debited from the mother city at dispatch,
-- rides on the unit (which is interceptable and destructible like anything else
-- on the map), and is credited to the colony at founding — or back to whatever
-- settlement the unit walks into if the expedition turns around.
--
-- Full plan: temenos_sitos_magasin_plan.md, steg 3.

ALTER TABLE units ADD COLUMN carried_silver float8 NOT NULL DEFAULT 0
    CHECK (carried_silver >= 0);

COMMENT ON COLUMN units.carried_silver IS
    'Silver this unit physically carries (colonist purse). Debited from the '
    'sending settlement at dispatch, credited at founding or on return. Never '
    'created here — the only faucets are genesis and mines (B3).';
