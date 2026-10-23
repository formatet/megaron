# ORDER32 — varvskön räknar bara skepp

Kontrakt före kod, bas611da45a. Blockerar kedjegrinden där landrekrytering felaktigt stoppar spelarens sjötransport.

Problem: båda Recruit-kontrollerna räknar alla väntande TrainComplete, även landutbildning. Spelarsanning: högst tio skepp får byggas i staden; landutbildning använder inte varvskön. Invariant: nya varvsjobb och legacy TrainComplete för samma skepp får aldrig räknas två gånger; avvisad order ändrar inga resurser/enheter.

Scope: Recruit-köfrågan i tidig och transaktionslåst kontroll, riktiga HTTP/PG-integrationstester. Befintliga samtidighetsgränser, arbetsmodell, betalning och eventsemantik består. Inga nya yttexter: samma serverbesked når webb och Keryx; Codex beskriver redan varvet. Stopp vid behov av ny kö-/rekryteringsregel.

Acceptans: tio riktiga landjobb hindrar inte ett skepp; tio nya skepp nekar det elfte; tio legacy skeppsjobb nekar det elfte; nio nya skepp med kvarvarande gamla timers tillåter exakt ett till. Bevis: namngiven röd gammalkodsbaslinje på färsk205, riktad grön, fysisk mutation/röd/byteexakt återställning, full fresh EN gång + vet. Ingen bild behövs för oförändrad presentation.

## Resultat

Runtime `6a2375e0`, slutlig fixtur `b3959128`; rebasad på master7ece841e (skillnaden från beställd611da45a var ORDER31-dokumentation). `navalBuildQueueCountSQL` används i båda kontrollerna. Den räknar skeppsenheter med pågående bygge, där sparat varvsarbete eller en aktiv gammal timer är alternativa vittnen. Gamla timers måste matcha unit_id, värld och stad; land, föräldralösa, färdiga och misslyckade timers tar inga platser. Inga migreringar eller skrivvägar ändras.

- **Gammal kod röd:** `TestRecruitShipQueueIgnoresTenLandTrainingJobs` gör tio riktiga autentiserade landrekryteringar och ser sedan felaktigt422 för skepp. `TestRecruitShipQueueDoesNotDoubleCountConvertedNavalTimers` visar felaktigt18 pending för nio skepp. `baseline.log`.
- **Slutlig riktad grön:** de namngivna testerna, tio nya/legacy skepp nekar exakt det elfte utan resurs-/populations-/enhetsändring, inaktiva timers, befintlig samtidighetskontroll och varvets livscykeltester. `targeted-green.log`.
- **Fysisk mutation:** tog bort remaining-vittnet så nya skepp slutade räknas; två kötester röda (`mutation-red.log`), byteexakt återställning dokumenterad med SHA256; slutlig riktad körning grön.
- **Fixturrättelse:** den första utökade riktade körningen lämnade syntetiska legacy-skepp utan sin historiska deadline och störde varvets globala deploykonvertering (`fixture-failure.log`). Fixturen har nu giltig deadline och lämnar inte kvar aktivt legacy-arbete efter scenariot. Ingen ändring av deploykoden; omkörning grön.

Fyra ytor inventerade: serverns befintliga fel säger korrekt ”build queue would overflow: 10 pending + 1 new > 10”; webb och Keryx använder samma Recruit-svar. Codex skiljer redan hela landkohorter från skeppsbyggen. Inga nya texter, notiser eller bilder behövs. Autentiserade HTTP-scenarier med verklig PostgreSQL, inte bara speglad räkningsaritmetik. Legacy-/felstatus är uttryckliga SQL-fixturer; ingen full naturlig resa från nygenererad värld påstås. Ingen live-skrivning eller deploy.

Full fresh och vet pågår; resultat kompletteras innan handover.
