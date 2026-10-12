# Spelarprofil — fem rader

Inventering före kod på **f57283cf**, gren `codex/spelarprofil`, eget worktree `/home/tk/wt/codex-spelarprofil`.
Claude integrerar; hash + STOPP.

| Kontrakt | Innehåll |
| --- | --- |
| Problem | Account saknar en egen sammanfattning av den aktiva världen. |
| Spelarsanning | Fem rader: Wanax, Since day N/Not yet founded, Cities, Population, Kharis. |
| Invariant | Endast inloggad Wanax i vald värld; inga motståndare, rang, totalsumma över världen eller historik. |
| Scope | Ny egen world-read om den saknas, Keryx me, account-window, Codex, prov/rigg. |
| Non-scope | Schema/balans, kontostatistik över alla världar, nova muterande verb/Umami-event. |
| Acceptans | Owner/world-filtrerade aggregat, fallen/Host utan500; nullable founded, full fresh Go/JS/vet, ownermutation, Firefox1280/390. |
| Stopvillkor | Founded-kolumnens semantik avstäms innan etiketten bindas till data. |
| Bevis | Inventering/baslinje, två-Wanax persistent API-prov, fysisk ownerfiltermutation, faktisk web/CLI/API och BILD. |

Kedjegrind: bevisar nåbar egen läsyta; ingen ny ekonomisk progression.

## Inventering före implementation

- **Endpoint saknas:** route-sökning i server/cmd/server/main.go och handlers hittar /auth/me (main296), ingen /worlds/{worldID}/me. CLI fetchMe är auth-identitet, inget me-kommando. Ingen parallell profilendpoint skapas.
- **Namn:** players.wanax_name, befintlig egen auth-identitet använder COALESCE(wanax_name,username) (auth/service.go207). Detta kan återanvändas endast för anroparen.
- **Grundning:** pwr.founded_tick (mig132) är NOT NULL DEFAULT0, stämplas på huvudstadsgrundning i create_metropolis.go188–201. Rise skapar ny founder_phase och nollar dess nullable founded_tick (nomadic_host.go190–205); nuvarande nådefrist är inte ett historiskt förstagrundningsaggregat. Null för og rundad founder behöver egen fas-vittnesrad, inte tick0 som gissning. Avstämning med Claude pågår.
- **Städer/befolkning:** settlements.owner_id + world_id; endast egna rader. Inga citynamn eller fremmande statistikrader behövs, ingen Host-befolkning räknas som stadsbefolkning. Fallen kan ha0 stadsrader; SUM måste coalescas0.
- **Kharis:** player_world_records per player_id/world_id. Befintlig loadPlayerKharis (handlers/db.go198) äger settled-läsning och templecap; använd dess amount, inte settlementresources eller rå lagrad lazy-mängd.
- **Account:** ui/account_window.js visar password/agora/delete-note/signout. accountGeneration/close-guard skyddar sena secrets; profil behöver samma guard plus världs-id så gammalt svar inte skriver in en annan värld. Inget nytt klick eller telemetryevent.
- **Codex:** community-chat är befintliga Account-ingången; den länkas till en kort account-artikel om de fem egna raderna. Keryx me får samma DTO, text/--json.

FOW-underlaget läst i sin helhet: bara owner_id-summor, ingen annan Wanax eller rank. Kontots loginname används aldrig för andra.

## Grindar

Pågår. Ingen migration planerad, endast en ny egen läsyta och presentation.
