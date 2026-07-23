-- Migration 096: klampa enheter som `divine_recruits` blåst upp
--
-- Välsignelsen `divine_recruits` (internal/kharis/tick.go) växte sitt mål med
-- 20 % utan tak, och eftersom den alltid väljer den STÖRSTA spearman-garnisonen
-- förstärkte den samma enhet varje gång — sammansatt tillväxt. Med kharis
-- fastnaglad vid taket 100 utlöstes välsignelser oavbrutet (315 DivineBlessing
-- på 12 h), så enheterna sprang iväg exponentiellt.
--
-- Mätt 2026-07-23: fem garnisons-spearmen på 1,86–2,13 MILJARDER man, mättade
-- mot int32-taket 2 147 483 647. En sjätte, size 2 976 790, hann grunda kolonin
-- Phaistos och myntade därmed 31,2 M silver — 99,5 % av världens silverstock.
--
-- Bara `spearman` överskrider taket, vilket är precis vad välsignelsen siktar
-- på: ingen legitim enhet i drift är större än 100. Taket i koden är
-- economy.MaxUnitSize och har alltid gällt vid rekrytering — välsignelsen var
-- enda skrivaren som saknade det. Den är nu bunden med LEAST().
--
-- OBS: det redan myntade silvret i Phaistos rörs INTE här. Att radera silver ur
-- en spelares stad är ett spelbeslut, inte en datafix — se megaron_todo.

UPDATE units
   SET size = 100, updated_at = now()
 WHERE size > 100;
