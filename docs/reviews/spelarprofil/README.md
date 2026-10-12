# Spelarprofil — fem rader

Inventering före kod på **f57283cf**, gren `codex/spelarprofil`, eget worktree `/home/tk/wt/codex-spelarprofil`.
Claude integrerar; hash + STOPP.

| Kontrakt | Innehåll |
| --- | --- |
| Problem | Account saknar en egen sammanfattning av den aktiva världen. |
| Spelarsanning | Fem rader: Wanax, Since day N/Not yet founded, Cities, Population, Kharis. |
| Invariant | Endast inloggad Wanax i vald värld; inga motståndare, rang, totalsumma över världen eller historik. |
| Scope | Ny egen world-read om den saknas, Keryx me, account-window, Codex, prov/rigg. |
| Non-scope | Schema/balans, kontostatistik över alla världar, nya muterande verb/Umami-event. |
| Acceptans | Owner/world-filtrerade aggregat, fallen/Host utan500; nullable founded, full fresh Go/JS/vet, ownermutation, Firefox1280/390. |
| Stopvillkor | Founded-kolumnens semantik avstämd med Claude 12:20. |
| Bevis | Inventering/baslinje, två-Wanax persistent API-prov, fysisk ownerfiltermutation, faktisk web/CLI/API och BILD. |

Kedjegrind: bevisar nåbar egen läsyta; ingen ny ekonomisk progression.

## Inventering före implementation

- **Endpoint saknas:** route-sökning i server/cmd/server/main.go och handlers hittar /auth/me (main296), ingen /worlds/{worldID}/me. CLI fetchMe är auth-identitet, inget me-kommando. Ingen parallell profilendpoint skapas.
- **Namn:** players.wanax_name, befintlig egen auth-identitet använder COALESCE(wanax_name,username) (auth/service.go207). Detta kan återanvändas endast för anroparen.
- **Grundning:** pwr.founded_tick (mig132) är NOT NULL DEFAULT0, stämplas på huvudstadsgrundning i create_metropolis.go188–201. Rise skapar ny founder_phase och nollar dess nullable founded_tick (nomadic_host.go190–205); nuvarande nådefrist är inte ett historiskt förstagrundningsaggregat. Null för ogrundad founder behöver egen fas-vittnesrad, inte tick0 som gissning. Claude bekräftade 12:20: Since day gäller nuvarande livet, aldrig första grundningen någonsin. Null när egen founder_phase.founded_tick saknas, även återuppstånden Host; äldre realm utan founder_phase visar pwr-kolumnens 0.
- **Städer/befolkning:** settlements.owner_id + world_id; endast egna rader. Inga citynamn eller främmande statistikrader behövs, ingen Host-befolkning räknas som stadsbefolkning. Fallen kan ha0 stadsrader; SUM måste coalescas0.
- **Kharis:** player_world_records per player_id/world_id. Befintlig loadPlayerKharis (handlers/db.go198) äger settled-läsning och templecap; använd dess amount, inte settlementresources eller rå lagrad lazy-mängd.
- **Account:** ui/account_window.js visar password/agora/delete-note/signout. accountGeneration/close-guard skyddar sena secrets; profil behöver samma guard plus världs-id så gammalt svar inte skriver in en annan värld. Inget nytt klick eller telemetryevent.
- **Codex:** community-chat är befintliga Account-ingången; den länkas till en kort account-artikel om de fem egna raderna. Keryx me får samma DTO, text/--json.

FOW-underlaget läst i sin helhet: bara owner_id-summor, ingen annan Wanax eller rank. Kontots loginname används aldrig för andra.

## Resultat och prov

- Server: GET `/api/v1/worlds/{worldID}/me`, Bearer-identitet, exakt `wanax_name`, `founded_tick`, `cities`, `population`, `kharis`. Inget query-/pathval av annan Wanax. Aggregerar bara egna aktiva städer i vald värld, samma urval som befintlig stadsöversikt. Kharis läses med befintlig lazy-helper.
- Keryx: `me` skriver fem läsbara rader, `me --json` bevarar API-fälten. Intern nullable tick exponeras som Since day N/Not yet founded, verklig grundning på tick0 bevaras.
- Webb: klick på Wanax-namnet öppnar befintliga Account med fem rader över lösenordsfälten. Generation och vald värld skyddar sena svar. Fel visas som fel, inte fabricerade nollor. Ingen ny Umami-händelse.
- Codex: ny account-artikel och länk från community-chat. Ingen migration, ingen ändring av grundnings-/rise-regler eller balans.

Persistent API-prov täcker två Wanaxer i samma värld, B:s ändrade städer/namn/kharis påverkar inte A; annan värld/icke-aktiva ruiner utesluts, query player_id ignoreras. Fallen, ny/återuppstånden Host, riktig tick0, äldre realm utan founder_phase, egen namn-fallback, 401/400/404 och lazy-kharis täcks. Fysisk borttagning av owner-filtret gav namngivet rött `TestWorldMe_OwnWorldIsolation` (3 städer/2400 istället för egna2/1600); byteåterställning gav färskt grönt. Se [mutation](owner-mutation.log), [återställning](owner-restored.log).

Baslinje: 598 JS + färska riktade Go. Efter ändringen: 602 JS gröna; riktad CLI/Account och fresh API gröna. Full färsk Go mot tom DB/migration173 efter fixture-fixen4d2bc50f och full vet gröna. Första fullsviten upptäckte att vårt nya prov lämnade en egen stad i en annan värld, som Agoras globala pass hittade. Fix4d2bc50f städar bara provets egna världar och konton; ingen Agora-produktionskod ändras. Ursprungliga röda körningen behålls för insyn.

## Riktiga ytor och BILD

Befintliga labbkonton **Lykoros** (Eirene, grundad) och **Eumedes** (ParityNomad, ogrundad Host) i aktiv värld `3b5fb9cd-8c06-4685-88df-b162c9acff99`. Ingen SQL-skrivning, request-/payloadinterception eller FOW-override under provet. API + nybyggd CLI + verkligt klick på Wanax-namnet i Firefox; varje fönster har exakt fem rader som matchar sitt faktiska API-svar. Inga JS-fel, profilen/panelen ryms vid1280/390. Befintlig labbdata kommer från tidigare acceptansslicers; detta är läsbevis, inte ett nytt naturligt grundnings-/ekonomiförlopp.

| Fall | Firefox1280 | Firefox390 |
| --- | --- | --- |
| Grundad Lykoros | [bild](founded-desktop.png) | [bild](founded-mobile.png) |
| Ogrundad Eumedes | [bild](host-desktop.png) | [bild](host-mobile.png) |

CLI: [grundad](founded-cli.txt), [Host](host-cli.txt). [Verification](verification.json) innehåller faktiska API/CLI/browser-fält och serverade asset-SHA; inga tokens sparas. Nybyggd riggprocess `23efbe2`, migration173. Byggmetod: CGO=0 lokal Go-binär ovanpå cachad isolerad acceptansruntime, nuvarande migreringar, eget worktree web-mount; ingen ren basimagebuild. [Runtime](rig/runtime.json), [provenance](rig/provenance.txt). Acceptance.sh väljer automatiskt den nyaste av tre världar (arkiverad `2e335492`); det riktiga profilprovet väljer uttryckligen den aktiva `3b5fb9cd`.

Repro: `PLACE_PROOF_OUTPUT="$PWD/docs/reviews/spelarprofil/rig" python3 tools/place_names_rig.py`, sedan `python3 tools/profile_proof.py`. Claude granskar kärna/bilder och integrerar; ingen merge/push/deploy från Codex.

Kontrollkommandon: `tools/gotest.sh ./...`, `env -i HOME="$HOME" PATH="$PATH" go -C server vet ./...`,
`node --test $(rg --files web/static/js | rg '\.test\.mjs$')`.
Owner-mutation: i world_me.go ersattes aggregatets WHERE med `world_id = $1 AND state = 'active'` och
argumentet med endast worldID (giltig SQL utan ägarfiltret). Färsk `tools/gotest.sh ./api/handlers -run TestWorldMe_OwnWorldIsolation`
gav felaktiga främmande siffror, sedan återställdes filen byte för byte och samma prov blev grönt.

Slutgrindar: kod/färska prov gröna; native Firefox läs-/geometribevis grönt; semantik följer Claude12:20; BILD lämnas till Claude/Timothy före merge. Ingen ny gameplay-handling, inga framställnings-/händelse-/balansregler ändrade.
