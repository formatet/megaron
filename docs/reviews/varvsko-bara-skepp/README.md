# ORDER32 — varvskön räknar bara skepp

Kontrakt före kod, bas611da45a. Blockerar kedjegrinden där landrekrytering felaktigt stoppar spelarens sjötransport.

Problem: båda Recruit-kontrollerna räknar alla väntande TrainComplete, även landutbildning. Spelarsanning: högst tio skepp får byggas i staden; landutbildning använder inte varvskön. Invariant: nya varvsjobb och legacy TrainComplete för samma skepp får aldrig räknas två gånger; avvisad order ändrar inga resurser/enheter.

Scope: Recruit-köfrågan i tidig och transaktionslåst kontroll, riktiga HTTP/PG-integrationstester. Befintliga samtidighetsgränser, arbetsmodell, betalning och eventsemantik består. Inga nya yttexter: samma serverbesked når webb och Keryx; Codex beskriver redan varvet. Stopp vid behov av ny kö-/rekryteringsregel.

Acceptans: tio riktiga landjobb hindrar inte ett skepp; tio nya skepp nekar det elfte; tio legacy skeppsjobb nekar det elfte; nio nya skepp med kvarvarande gamla timers tillåter exakt ett till. Bevis: namngiven röd gammalkodsbaslinje på färsk205, riktad grön, fysisk mutation/röd/byteexakt återställning, full fresh EN gång + vet. Ingen bild behövs för oförändrad presentation.
