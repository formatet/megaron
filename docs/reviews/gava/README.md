# S — gåva och tribut

Kontrakt före kod, bas eb523aa2. Timothy har beslutat att Transfer ska kunna
skicka varor och silver till en annan Wanax utan motprestation eller samtycke.
Mottagaren måste klara samma brev-FOW-grind. Godset går i fysisk rånbar
karavan/skepp. Intern transfer behåller sina regler; silver är en vara.

Spelaren möter detta i Economy → Transfer och leveransraden i Diplomacy,
keryx transfer och Codex. Nya event/notistyper bär gåvans utfall till båda.
Debit, mover, timer och utfall ska vara atomiska/idempotenta; inga dubbla
krediter eller skepp som blir bundna utan aktiv resa. Befintliga eventtyper
omtolkas inte.

Scope: transfer-handler/FOW/ägarlås, gåvans ankomstutfall, fysisk transport,
CLI/webb/notisformat/Codex, migration om gåvans mottagare/historik behöver
bestående identitet. Ingen kredit, skuld, tributautomation eller handelsbalans.
Bevis: främmande transfer röd→grön, dold stad avvisad utan debit, normal och
rånad leverans, ägarbyte/kollaps/tak/ships release, färsk full Go/vet/JS,
FOW/kredit/intern-förlustmutationer, två Wanaxer via spelarytor och 1:1 BILD.
BILD lämnas till Timothy via Claude; Codex gör ingen merge/push/deploy.

Kanonberoende utfall är obeslutade: mottagarbyte under resa, fallen stad,
fullt lager/överskott. Fråga skickad till Claude innan leveransregeln byggs.
FOW-repro/inventering fortsätter oberoende. Gåvan stödjer kedjans bronsutbyte;
slice bevisar den beslutade gåvans nåbarhet, ändrar inte produktion/balans.

## Baslinje och oberoende del

`baseline.log`: oförändrad eb523aa2, färsk PG16/migration160; befintliga interna
transfer-/karavanprov gröna. `baseline-js.log`: 433 gröna tester.
`red.log`: riktig handler/DB, silver till annan Wanax i brevets synradie ger
**403** own-only (ordern angav 400); nytt acceptansprov väntar 201 och är rött.
Dold stad avvisas utan debit eller transport.

Kanonoberoende GET `/provinces/{provinceID}/trade/destinations` listar egna och
främmande aktiva mottagarstäder med stad-id/namn och Wanax-id/namn. Källa måste
vara egen aktiv stad. Främmande städer prövas med exakt samma
`loadVisibleOrigins` + `province.VisibleFrom(...,6)` som brev. Dolda namn/id:n
lämnar aldrig endpointen. `destinations.log`: färskt contacted/hidden-prov grönt.
`mutation-destination-list-fow.log`: släpp igenom främmande stad utan FOW →
hidden-listprovet rött. `destinations-restored.log`: återställd kod grön.

Intern transfer är redan fysiskt rånbar: province.go DispatchParams
Interceptable=true och befintligt internal-transfer-prov kontrollerar detta.
Undantaget gäller förlusttärningen, inte interception. Den byggda regeln bevaras;
mutation av intern risk avser storm/pirates-tärningen. Claude är underrättad.

Timothys tillägg via Claude: nya gåvohändelser ska bära både sändande och
mottagande Wanax-id och stad-id så bedrifter kan följas. Ingen relationsliggare
eller summering byggs. Kanonfrågorna om ägarbyte/kollaps och överskott är ställda
till Timothy och Claude. Leveransimplementation väntar på de två svaren.
