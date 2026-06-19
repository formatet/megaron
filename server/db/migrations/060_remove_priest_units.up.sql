-- Migration 060: ta bort prästen som ENHET.
-- Låst invariant: "präst är ingen enhet längre — kult = tempel-labor; inga
-- prästenheter (varken byggbara eller starter)." Prästenheter kunde ändå skapas
-- via recruit (UnitSpecs hade en "priest"-post). Posten är borttagen i koden
-- (recruit 400:ar nu "unknown unit type"); detta städar bort de rader som hann
-- skapas. Präster bidrar 0 fältstyrka (resolver.go) → säkert att radera.
-- De legacy `priest`-HELTALSKOLUMNERNA (settlements/marching_armies/borrowed_armies)
-- rörs INTE här — de hör till den separata dual-write-kolonn-droppen (D-stream).
DELETE FROM units WHERE type = 'priest';
