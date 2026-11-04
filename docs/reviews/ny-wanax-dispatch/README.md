# "A new Wanax has entered the Thalassa" (Sonnet 2026-11-04)

Order `.agents/order-ny-wanax-dispatch-20261104.md`; beslut Timothy 2026-11-02.

## Kontrakt som byggts
- `POST /founding/settle` tar valfritt `message`. Trimmas; **över 280 tecken (runes) nekas med 400 före allt annat** (grundandet är oåterkalleligt, tyst klippning vore fel — se fråga 1). Tomt = dispatch utan citat.
- Fula ord: `cleanLetter` (samma som brev) + `angerGodsTx(..., "letter")`, separat från namnets `"name"`.
- `announceWanaxEntered` (`handlers/wanax_entered.go`) i grundningens tx: event `WanaxEntered` (strömmen world) payload `{wanax_id, wanax_name, message}` + en notis (nivå 3, kind `WanaxEntered`) per annan Wanax med `player_world_records` i världen. Ingen kopia till grundaren. Namn = `wanax_name`, annars `username`. **Ingen position, inget stadsnamn.**
- "Första": `founder_phase.is_refugee = false`. Återgrundning efter fall går via `rise` (seedHost sätter `is_refugee=true`) och kungörs inte. `Settle` hindrar redan en andra stad per Wanax (`errAlreadySettled`); kolonier går via `foundColony` och når aldrig funktionen (grep: enda anropet är `Settle`).
- Idempotens: mig 243 unikt delindex `(world_id, player_id, body_json->>'wanax_id') WHERE kind='WanaxEntered'` + `ON CONFLICT DO NOTHING`. **Mig 243 hoppar över 241–242 (reserverade för annan byggare)** — kolla att golang-migrate tål luckan innan merge.
- Fyra ytor: temenos (ovan) · keryx `founding settle --message`, notisdetalj i `notification_text.go` · webb: textfält i Host-panelen (`map.js`, klass `.ip-settle-msg`), `format.js` text/ikon/kategori; Umami-eventet `settle` kommer redan från verb-katalogen (inget nytt event) · Codex `a-new-wanax.md` (+ index.json) och en rad i `founding.md`.
- CLAUDE.md: fjärde undantaget tillagt. Vault `megaron_verblista.md`: raden uppdaterad.

## Bevis
`wanax_entered_test.go` (5 tester, HTTP-nivå via Join+Settle): en notis per annan, ingen till grundaren, payload-nycklar exakt `message,wanax_id,wanax_name` och JSON utan stadsnamn/`q`/`r` (även eventet) · refugee → ingen · dubbel announce → inga dubletter · 281 tecken → 400 och ingen stad · fula ord ersätts och debiteras (1 GodsAngered) · tomt meddelande. Keryx-test, två webbtester.
Mutanter (alla dödade): refugee-grinden bortkopplad → `NotToldForAReturningWanax`; `"q"` i payload → `FirstFoundingToldToAllOthersWithoutPlace`; skicka även till grundaren → samma test; `cleanLetter` borttagen → `MessageLimitAndCoarseWords`; `ON CONFLICT` borttagen → `AnnouncingTwiceMakesNoDuplicates`. Koloni-mutanten ("skicka även vid koloni") kan inte skrivas som test: kolonivägen anropar aldrig funktionen (strukturellt skydd, ingen testbar gren).

## Öppna frågor
1. Längdtak: nekas (400) i stället för att klippas tyst. Ändra om Timothy hellre vill klippa.
2. Webbfältet visas också för en återvändande Wanax (statusen exponerar inte `is_refugee`); meddelandet ignoreras då tyst. Kan döljas om `is_refugee` läggs i `founding/status`.
3. Mottagare = alla med `player_world_records` i världen (inkl. dispossessed/AI), inte de som går med senare.
4. BILD efter merge: fältet i Host-panelen och notisen i dispatch-fönstret (Firefox). TEXT i playtest: formuleringen.
