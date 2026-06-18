-- Best-effort reverse: recreate the four columns and copy the silver row amount back.
ALTER TABLE settlements
    ADD COLUMN silver_amount  double precision NOT NULL DEFAULT 0,
    ADD COLUMN silver_rate    double precision NOT NULL DEFAULT 0,
    ADD COLUMN silver_cap     double precision NOT NULL DEFAULT 1000,
    ADD COLUMN silver_calc_at timestamptz      NOT NULL DEFAULT now();

UPDATE settlements s
SET silver_amount  = COALESCE(
        (SELECT settled(amount, rate, calc_at)
         FROM settlement_goods sg
         WHERE sg.settlement_id = s.id AND sg.good_key = 'silver'), 0),
    silver_calc_at = now();
