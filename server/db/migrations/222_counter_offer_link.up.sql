-- The new offer's terms own its parent link; the guarded old-offer transition
-- serializes replacements. This index is a second backstop against two children.
CREATE UNIQUE INDEX messengers_one_counter_offer
 ON messengers((trade_offer->>'counter_offer_id'))
 WHERE trade_offer->>'counter_offer_id' IS NOT NULL;
