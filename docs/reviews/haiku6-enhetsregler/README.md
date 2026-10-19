# Haiku 6 (b) — enhetsregler

Kontrakt före kod: verifiera tre rapporterade motsägelser innan ändring:
marschhjälpens 100 man mot en marscherande 64-mannakohort, nekad 40-mannakohort,
och reinforce som säger marchera hem fast spelaren uppfattar enheten som hemma.
Spelarsanning: nyrekrytering som ännu formas skiljs från en färdig kohort efter förluster;
rekryteringshem är det mekaniska hemmet för förstärkning.
Invariant: ingen ändring av storleksgränser, rekrytering, förluster eller förstärkningsvillkor.
Scope: reproduktionsbevis och endast motiverade text-/ytfixar i server/keryx/web/Codex.
Acceptans: verkliga tillstånd avgör varje fynd; felaktig hjälp rättas; reglerna oförändrade;
relevanta tester gröna; native användarprov för ändrad text där den möts.
Stopp: en önskad ny regel kräver beslut från Timothy, inte en tyst balansfix.
Bevisplan: baslinje från färsk databas, HTTP och riktig CLI; mutation av textkontrakt;
fyra ytor inventeras. Kedjegrind: bevisar enheters handlingsbarhet på vägen till elit.

## Resultat

Kod/text `d03672ab`. Endast felaktig marschhjälp och en serverkommentar rättas;
webbens rekryteringshint och Codex preciserar den befintliga skillnaden mellan
ursprungsstaden och andra egna städer. Ingen regel, storleksgräns, API-payload,
eventsemantik, migration eller felsträng ändras.

| Rapport | Verifiering | Åtgärd |
|---|---|---|
| Hjälpen kräver 100 men en 64-kohort marscherar | CLI före: `A land unit must reach 100 men (garrison status) before it can march.` HTTP med färdig garrison64 accepterar explore202 och lagrar marching64. | Hjälp rättad: forming/training väntar; färdig kohort kan marschera efter förluster. |
| 40-mannakohort nekades | Seedad forming40 nekas422 med explicit statusförklaring och förblir forming40. | Ingen storleksfix. Originalets exakta status finns inte i underlaget; forming-repro är ett möjligt, verifierat tillstånd, ingen fastställd förklaring till den historiska ordern. |
| reinforce nekas trots hemma | HTTP med garrison64 och settlement=origin ger200. Garrison62 i annan egenstad ger422, `march it back`, oförändrad reinforcing=false. | Mekanikbugg ej reproducerad, släpps. Codex/webb gör ursprungsstad tydlig; CLI reinforce-hjälpen angav redan staden som reste kohorten och lämnas. |

Färsk PG/HTTP-fixtur betyder här seedade tillstånd genom riktig autentiserad handler,
inte ett påstående om att Haikus historiska strid/rekrytering återskapats.
Detaljer och exakta befintliga fel i [server.md](server.md).

## Grindar

Hela Go-sviten på ny PG16/189 är grön (`full-go.log`, inklusive world 150.579 s);
`go vet ./...` är grön (`vet.log`).

- Baslinje: keryx-paketet grönt (`cli-baseline.log`), 24 relevanta JS-test på oförändrad
  masterkopia gröna (`js-baseline.log`). `march-help-before.txt` kommer från en byggd
  oförändrad 07e2eac4-binär och verkligt `unit march --help`.
- HTTP: serveragentens färska PG189-baslinje, riktade kontroll och handlers-svit gröna
  enligt server.md. Dess råa stdout sparades inte separat; förälderns arkiverade
  mutations-/slutsvitsloggar nedan bär det bestående körbeviset.
- Mutation: `python3 docs/reviews/haiku6-enhetsregler/mutate.py` lägger fysiskt till
  en påhittad 100-mansgrind. Det nya HTTP-fallet för färdig64 blir namngivet RED
  (`mutation-red.log`), byte-exakt återställning följd av ny färsk PG blir GREEN
  (`mutation-restored.log`). Produktionslogik återställd före kodcommit/slutsvit.
- Webb/Codex: 24 relevanta tester samt hela oförändrade familjen **650/650 gröna**
  (`js-final.log`, `full-js.log`). Färgbranchens två extra kontrasttester finns inte
  i denna bas; den grenen väntar BILD/integration hos Claude.
- Native textyta: två helt nya naturliga register → verklig CLI login/help/codex/join/found
  → Firefox War → Recruit. Ingen SQL-skrivning, FOW-manipulation eller API-replay.
  Båda healthz: **d03672ab8c2536523a5e9f52389c8c2a3934de6a**, migration189, status ok.
  `native-a/proof.json`, `native-b/proof.json`, CLI-utdata och skärmbilder visar
  den faktiska ändrade vägledningen på 1280/390, DPR1; inga pageerrors eller overflow.
  Dessa provar textens nåbarhet; förlust-/forming-tillstånden provas separat via HTTP.
- Semantik: fyra ytor genomgångna; serverreglerna oförändrade, keryx-hjälpen korrigerad,
  webbhinten och Codex överens om färdig kohort respektive ursprungsstad.
- TEXT: begripligheten ska prövas av nästa spelare som möter vägledningen, genom Claude.
  Builderns 1:1-kontroll visar ingen trunkering men är inte ett oberoende lästest.
- Ingen merge/push/deploy eller liveåtgärd. Claude granskar och integrerar.

## Reproduktion

```sh
tools/gotest.sh ./api/handlers/ -run 'Test(Reinforce_|March_ExploreCompleted64AndForming40)' -v
python3 docs/reviews/haiku6-enhetsregler/mutate.py
python3 docs/reviews/haiku6-enhetsregler/native.py /tmp/haiku24b-new-proof
```

Baslinjens fullständiga arkiv behövs: `git archive 07e2eac4` (inte bara web/static,
eftersom Codex-tester även läser templates och serverfiler). Ingenting i detta arbete
fastställer orsaken till Haikus äldre hemstadsmotsägelse utan det ursprungliga tillståndet.
