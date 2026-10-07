# I — War-kortet

Kortet blandade huvudorder med sekundära kontroller och saknad ETA blev
Invalid Date/NaNm. Nu står **March** (och berättigad **Reinforce**) framme i
stad/fält; marscherande land visar **Recall**. Stance, battle retreat, Fetch,
Redirect med Q/R, Load/Unload och Repair ligger under stängd **More**. Alla
förmågor och orderbodies är kvar. Status och missiondygn visas i spelarord.

Bas: **master93759a46**, egen gren `codex/forenkling-war`. Produktionskod
**ed48a74a**. Slutlig beviscommit = grenens HEAD. Bevisar geografikedjans nåbara
order. **BILD granskas av Claude** enligt Timothys delegation. Ingen
merge/push/deploy. Stanna efter hash; Claude ger nästa slice uttryckligen.

| Kontrakt | Innehåll |
|---|---|
| Spelarsanning | Huvudorder följer status; sekundära tillåtna order finns under More; saknad ETA tom. |
| Invariant | Samma API/body och regler; alla retreatpresets kvar; FOW och serverport-regler bevaras. |
| Scope | war.js, time.js, prosahelpers, motsvarande Codexinstruktioner och prov. |
| Non-scope | Backend/keryx, kanon, J/K, realm-wide retreatformulär. |
| Acceptans | Stängd More; rätt primär order; sekundära order nåbara; skepp vid havet följer R3 + R6; giltig tick vinner över saknad/ogiltig ISO. |
| Stopvillkor | Regeländring till Claude; ingen behövdes. |
| Bevisplan | Oförändrad baslinje, desktop390, statematris/NaN-prov, fysisk mutation, färska vanliga webbflöden och read-only audit. |

| Grind | Bevis |
|---|---|
| Baslinje | **health93759a46/mig160**, arkiverade oförändrade assets. 390 JS gröna; nytt NaN-prov namngivet rött. Riktig sentry/fortify/clear → March → Recall → garnison, audits i `live-baseline/proof.json`. |
| Kod | **400 JS gröna**, full fresh PG16/mig160 Go-svit (world91.835s), full `go vet ./...` exit0. `logs/`; server/keryx-diff mot basen **tom**. |
| Mutation | NaN-vakt, stance under More och navalrecall-vakt fysiskt brutna → varsin namngiven assertion röd → restored grön. `mutations/`. |
| Användare | Två färska gröna landresor (först healthd2b66ca1, sist **healthed48a74a/mig160**): More öppnas, stance×3, vanlig kart-March, More→Redirect→typed Q/R nåbara, Recall202, en march-audit/en recall-audit, enheten hemma. `live-after/`. |
| Skepp | Två separata färska kustgrundningar via vanliga joins: **healthed48a74a/mig160**, More→Load200→More→Unload200, samma cargo, lasten garrison och skeppet tomt. `live-naval/`. |
| BILD | Före/efter desktop/390px, stängd/öppen More, stad/marsch + naval load/unload. Kortet ryms, inga browserfel. `live-baseline`, `live-after`, `live-naval`. Claude beslutar. |
| Drift | Ej deployad; egna riggar stängs av verktygen. |

`arrivalHTML`, `fmtArrival` och `fmtEta` avstår från ogiltig/saknad tid;
authoritative tick, giltig ISO-fallback och doneWord fungerar fortfarande. Ingen
lös "arrives"-text lämnas på kortet när ETA saknas.

En gammal War-knapp erbjöd March på positioned skepp med stad kvar, vilket R3
redan avvisar. Kortet följer nu `RequireShipInPort` och bevarar **R6** (ingen egen
stad ⇒ strandsatt skepp får order). En hull under byggnad visar inte längre
"out of food" som om den var till havs. Inga spelregler ändrades.

Avgränsning: faktisk battle retreat, Fetch och Repair körs inte i webbriggen;
statematrisen verifierar att kontrollerna/gates/alla val finns kvar, deras
POST-kod är oförändrad. Typed redirect öppnas men dispatchas inte. Den första
navalarmen körde även Explore i ett helt känt område och fick korrekt422;
navalbeviset avgränsades därefter till dess verkliga Load/Unload-uppgift.
Kanoniska retreatval och numeriska redigerbara fält/koordinater bevaras.
Ingen ny CSS/färg/inline-styling; äldre styling på flyttade kontroller behålls.

Reproduktion: [tools/README.md](../../../tools/README.md#war-kort-i).
`node --test 'web/static/js/megaron/**/*.test.mjs'`;
`python3 tools/war_cards_mutations.py OUT`;
`python3 tools/war_cards_live.py OUT <hash> baseline|after|naval [WEB_DIR]`.
Varje OUT har egen binär med rätt buildCommit; varje arm skapar egen tom DB/Redis.
