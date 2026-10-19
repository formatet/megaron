# Haiku 6 (a) — CLI-argument och vald transferkälla

## Kontrakt före kod

Problem: argument som spelaren tror betyder en order kan ge katalog, otydligt fel
eller felaktig källa i hjälpen. Spelarsanning: saknad byggnadstyp och obligatoriskt
arbetsplatsnamn ska framgå; kolonisering ska förklara bekräftelsen; en överföring
till samma stad som vald källa ska förklara källvalet, inte säga att staden saknas.
Invariant: ingen tyst order, ingen omtolkning som skickar varor från en annan stad,
ingen ändring av rapporterings-/koloniserings-/handelsregler eller informationssynlighet.
Scope: CLI-validering och hjälp, befintliga serverytors konsument, Codex och bevis.
Acceptans: alla fem fynd prövas; bara reproducerade fel rättas; fel skickar ingen POST;
korrekt explicit syntax fungerar; kolonin som vald källa behålls och --from fungerar.
Stopp: ny spelmekanik eller automatisk omtolkning av stad/byggnad kräver diskussion.
Bevis: verklig baseline-binär, HTTP-kontrakttester, riktiga CLI på färsk isolerad värld,
relevanta tester/fysiska mutationer och full slutkontroll. Kedjegrind: bevisar användbara
order och egen logistik i brist → brons.

Flaggformen `report --text` är avsiktlig enligt befintlig argumentpolicy; det ska
verifieras om det befintliga felet redan visar rätt kommando innan något ändras.
`staff` kräver arbetsplatsens namn; "default show" betyder utan delta, inte utan namn.
Transfer default hämtas faktiskt ur vald cfg.ProvinceID, inte automatiskt ur huvudstaden.
Ingen ny standardstad väljs i denna slice; hjälp och självöverföringsfelet ska tala sant.

## Resultat

Kod **f3a239ec** på bas **b3221f27**. Serverregler, webbformulär och loginval oförändrade.

| Fynd | Verifierat före → efter |
|---|---|
| `report "…"` nekas | Avsiktlig flaggform. Behålls; förslaget citerar nu texten så mellanslag och shelltecken överlever inklistring. `report --text "…"` skickar exakt en rapport med oförändrad text. |
| `build harbour` | Före: exit0 och katalog, ingen byggorder. Efter: exit1 med --type/--list-hjälp före HTTP. `build <city> --type …`, bar `build`, --list och --queue behålls. |
| `staff` | Före: generiskt RangeArgs-fel. Efter: obligatoriskt byggnadsnamn och hänvisning till city/roster; Short/Long och exempel anger namngiven arbetsplats, utan delta är det en läsning. |
| Kolonisering saknar --yes i hjälp | Flaggan förklarade redan kravet. De två landkoloniseringsexemplen kompletterade; inga bekräftelsevillkor ändras. Riktig rör-inmatning utan --yes visar prognos och skickar ingen order; med --yes grundas kolonin. |
| GPT9 transfer till vald koloni | Destinationerna exkluderar källan. Före: `no own or contacted city named "West"`. Efter: `sending city "West" is also the destination — choose another source with --from <city> (see keryx settlements)`. --from Myrsine lyckas; vald koloni ändras inte. |

Transferhjälpen sade felaktigt "default: your capital" medan koden använde vald stad.
Nu beskriver hjälp och Codex faktiskt källval. Vi väljer inte automatiskt en annan
avsändare; en skrivorder som spelaren inte avsåg vore värre än ett tydligt fel.

## Bevis och grindar

- **Baslinje:** oförändrade keryx-paketet grönt (`cli-baseline.log`); nya kontrakt blir
  namngivet röda för byggtyp, arbetsplats och self-transfer (`contracts-before.log`).
  Korrekt report-flagga var redan grön. Ursprungliga kodens stdout/stderr finns också
  från en verklig CLI-binär byggd ur ett komplett b3221f27-arkiv i nativeproven.
- **Slutkod:** hela Go-sviten på en ny PG16/189 grön (`full-go.log`, world 143.944 s); `go vet ./...`
  grön (`vet.log`), alla **650 JS-test** gröna (`full-js.log`), hela CLI-paketet grönt.
  Detta ändrar ingen DB-regel men hela slutkontrollen är körd enligt arbetsflödet.
- **Mutation:** `mutate.py` körs med ren HOME/PATH-miljö. Fyra fysiska mutationer
  (byggvalidering, obligatorisk arbetsplats, tvingad huvudstad, borttagen citering)
  ger namngivna RED → byte-exakt återställning → GREEN i `mutations.json`.
  CLI-kontrakten använder autentiserad httptest-HTTP och räknar läsningar/POST,
  inte en persistent databas. Shellkopieringsprovet prövar verklig argumentrundtur.
- **Native:** två nya världar med register → CLI login/join/found → verklig marsch
  för sikt → kolonisering → transfer. Healthz båda: **f3a239ecc457cef08668abb9b9e1d87f5fd42638**,
  migration **189**, status ok; proof/CLI i `native-a` och `native-b`.
  Gamla/nya binären ger före/efter på samma naturligt skapade stad och koloni.
  `binary-context.json` identifierar förebinärens SHA och call-index; råutdata orörda.
- **Konfiguration:** endast klientens valda province_id sätts till den naturligt
  grundade kolonin för att återskapa det rapporterade källvalet. Ingen SQL-skrivning,
  uppfunnen koloni, teleportering, FOW-fabrikation eller API-replay. Lokal tick=1s
  driver riktiga händelser; detta är acceptansvärldar, inte de levande GPT-spelarna.
- **Webbparitet:** Firefox DPR1/1280+390 visar befintlig Economy→Transfer med källan
  West och endast Myrsine som egen destination. Inga pageerrors; 1:1-bilder granskade.
  Webbformuläret behöver ingen ändring. CLI läser också de två ändrade Codex-artiklarna.
- **TEXT:** nästa spelare bedömer begripligheten. Koden bevisar validering och frånvaro
  av oavsiktliga POST; byggaren kan inte själv bevisa oberoende begriplighet.

Första nativeprobet antog felaktigt att ärvd stdin var en pipe; den var en char-device
så CLI erbjöd bekräftelse och avbröt på EOF (`initial-native-check-failed.log`). Probet
sätter nu riktig rör-inmatning. Första kandidathexen låg dessutom för nära staden;
slutprovet marscherar för sikt och väljer en tillåten plats längre bort. En API-nyckel
för centrumkoordinater rättades till faktiska `Q`/`R`. Dessa var riggfel, inte produktfel.
Båda slutkörningarna ovan börjar från helt nya världar med det rättade scenariot.

## Reproduktion och avgränsning

```sh
go -C server test ./cmd/keryx -run 'TestHaiku6|TestSuggestedArgumentSurvivesShellCopy' -v
python3 docs/reviews/haiku6-keryx-argument/mutate.py
# Bygg först en baseline-keryx ur ett komplett git archive b3221f27:
python3 docs/reviews/haiku6-keryx-argument/native.py /tmp/haiku24a-new /path/to/baseline-keryx
```

Inga liveanrop, inloggningspolicybyten, API-/spelregeländringar eller nya positionalalias.
Okända destinationsnamn har kvar det generella felet; bara den faktiskt valda egna
källan får självöverföringsförklaringen. --from, befintlig UUID-väg och kontaktade
främmande städer behålls. Ingen merge/push/deploy; Claude granskar och integrerar.
