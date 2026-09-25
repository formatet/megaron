-- Migration 144 down: medvetet no-op. Numren är korrekta data (samma som en ny
-- join skriver); att nolla dem skulle återskapa buggen, och upp-migrationen
-- skiljer inte längre ut vilka rader den själv satte.
SELECT 1;
