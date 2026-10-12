-- Labor is an offering reference, never an NPC price. Historical prices and
-- valuations are retained but no longer consumed by production code.
ALTER TABLE goods ADD COLUMN labor_value double precision CHECK (labor_value > 0);
COMMENT ON COLUMN goods.base_value IS 'Deprecated historical price; no runtime consumers after migration 177.';
COMMENT ON COLUMN goods.labor_value IS 'Worker ticks per unit on standard terrain; bronze includes its input chain. NULL for non-goods/unavailable production.';
UPDATE goods SET labor_value = CASE key
    WHEN 'stone' THEN 2 WHEN 'bronze' THEN 11
    WHEN 'cult' THEN NULL WHEN 'purple' THEN NULL
    ELSE 1 END;

-- Same building multiplier 1 + 0.7*level; L1 anchors are exactly 1 / 2.5.
UPDATE settlement_goods SET amount = GREATEST(0, settled(amount,rate,calc_tick)),
    calc_tick = current_world_tick(), rate = rate / 2.304
WHERE good_key='silver' AND rate > 0;
UPDATE production_rules SET rate_per_tick = CASE terrain_type
    WHEN 'hills' THEN 1.0/1.7 WHEN 'mountain_limestone' THEN 2.5/1.7 END
WHERE good_key='silver' AND building_type='mine' AND terrain_type IN ('hills','mountain_limestone');

-- A separate index prevents smoothing work into the old silver-price scale.
CREATE TABLE divine_labor_valuations (
    world_id uuid NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    good_key text NOT NULL REFERENCES goods(key),
    rarity_spread double precision NOT NULL,
    rarity_volume double precision NOT NULL,
    divine_value double precision NOT NULL,
    calc_tick bigint NOT NULL,
    PRIMARY KEY (world_id,good_key)
);
COMMENT ON TABLE divine_labor_valuations IS 'Valuation version 2, unit worker_ticks. Initialized by server before serving requests.';

ALTER TABLE founder_phase ADD COLUMN is_refugee boolean NOT NULL DEFAULT false;
-- Previous-city witness already distinguishes pre-migration returning hosts.
UPDATE founder_phase fp SET is_refugee=true FROM player_world_records pwr
WHERE fp.world_id=pwr.world_id AND fp.owner_id=pwr.player_id
  AND fp.active AND pwr.last_settlement_id IS NOT NULL;

-- Do not confiscate balances during cap replacement. No existing amount is
-- topped up; the endowment applies to future first founding only.
UPDATE settlement_goods sg SET
    amount=GREATEST(0,settled(sg.amount,sg.rate,sg.calc_tick)),
    calc_tick=current_world_tick(),
    cap=GREATEST(GREATEST(0,settled(sg.amount,sg.rate,sg.calc_tick)),
        CASE WHEN s.is_capital THEN CASE WHEN p.coastal THEN 2556 ELSE 1460 END
             ELSE GREATEST(60,2*GREATEST(0,settled(sg.amount,sg.rate,sg.calc_tick))) END)
FROM settlements s JOIN provinces p ON p.id=s.province_id
WHERE sg.settlement_id=s.id AND sg.good_key='silver';
