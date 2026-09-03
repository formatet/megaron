-- Down för 139: återställ tennet till kopparns värde och bronset till 20.
--
-- Tennet: före 139 var tenn och koppar prissatta exakt lika (båda 172,8),
-- eftersom migration 136 skalade varje varas base_value med dess egen
-- produktionstakt och därmed plattade ut den premie tennet hade före dess
-- (koppar 6, tenn 12). Down skriver därför tenn = kopparns LIVE base_value,
-- av samma skäl som up:en läser den live: ett hårdkodat 172,8 skulle drifta
-- isär från kopparn den dag någon rör den.
UPDATE goods SET base_value = (SELECT base_value FROM goods WHERE key = 'copper')
WHERE key = 'tin';

-- Bronset: 20 är ett exakt historiskt värde, inte en härledning. Migration 136
-- rörde aldrig brons (noll träffar på 'bronze' i 136_dagsverkesskalan.up.sql)
-- eftersom brons görs ur ett recept och därför saknade produktionstakt att
-- skala. 20 är alltså talet som stått orört sedan varan infördes, och det är
-- vad down ska återställa till.
UPDATE goods SET base_value = 20 WHERE key = 'bronze';
