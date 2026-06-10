ALTER TABLE kingdom_members
  ADD CONSTRAINT km_role_check
  CHECK (role IN ('basileus', 'member', 'lochagos', 'navarchos'));
