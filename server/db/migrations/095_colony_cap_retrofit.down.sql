-- Ned: återställ de gamla per-vara-taken som foundColony hårdkodade före 095.
-- Bara för rader som fortfarande står på det icke-bindande taket — rör inte
-- silver, vars tak är pop-skalat av Sitos-seeden.

UPDATE settlement_goods
   SET cap = CASE good_key
                 WHEN 'grain'  THEN 1000
                 WHEN 'stone'  THEN 1000
                 WHEN 'timber' THEN 500
                 WHEN 'cedar'  THEN 500
                 WHEN 'copper' THEN 300
                 WHEN 'tin'    THEN 300
                 ELSE 200
             END
 WHERE good_key <> 'silver'
   AND cap = 1000000;
