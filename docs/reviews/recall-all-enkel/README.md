# Recall all — enkel klientloop

Kontrakt före kod. Problem: återkalla flera egna marscher kräver ett klick/anrop
per enhet; grupperingsformen är förkastad. Spelarsanning: EN Recall all-knapp
och CLI recall --all skickar samma vanliga recall till varje marscherande egen
enhet, en kurir per enhet enligt serverns befintliga regler. Invariant: ingen
ny serverkod/rutt/filter/eventsemantik och inget tyst bortfall vid avslag.
Scope webbknapp/loop i War, CLI-flagga/loop, en Codex-mening, verblisterad och
prov/egen rigg. Non-scope parkerade aggregat-/batchgrenar, backend, ny design.
Acceptans: alla marscherande egna får försök, avslag namnges, en kort summa,
ingen aktiv knapp utan marscher, befintlig singel-recall bevarad; web/CLI
faktiska audits+garnison på varsin ny DB; tester/mutation/fullsvit/vet/JS gröna.
Stoppa vid canonkonflikt eller behov av serverändring (rapportera till Claude).
Bevisar kedjegrindens nåbara order/återkomstyta. BILD för Timothy senare,
TEXT i playtest. Bas masterb353d54f; egen gren codex/recall-all-enkel.

## SAMMANFATTNING

En Recall all-knapp och CLI recall --all loopar den BEFINTLIGA per-enhet-recall.
Bara egna marscherande enheter får försök; en kort summa och namngivna avslag.
Ingen serverrutt/-kod, gruppering eller filterflagga. Kod7f30b7e5, ingen deploy.
Riktiga webb/CLI-processers /healthz: commit7f30b7e5, migration160, statusok.

## BASLINJE

Faktisk egen tom PG16/Redis-rigg, healthd48a093f/mig160: all-knapp saknas och
CLI-hjälpen saknar --all; två vanliga Recall-klick gav varsin fysisk kurir,
varsin MarchRecalled-audit och garnison. [Proof](baseline/proof.json),
[bild](baseline/recall-desktop.png). Kodex/CLI-alias baslinje gröna.
Nytt CLI-prov före flaggan gav korrekt assertionrött unknown flag --all.

## EXPERIMENT OCH GRINDAR

| Grind | Bevis |
|---|---|
| Kod | Full tools/gotest.sh NY PG16/mig160 alla paket gröna inklusive world91.187s: [full-go.log](full-go.log). Full vet grön, 384 JS gröna. |
| Mutation | Båda riktiga källlooparna begränsade till FÖRSTA enheten: JS varje-marscherande-assertion och CLI HTTP-requestlista assertionröda, återställda tester gröna. [Webb](mutation-web-red.log), [CLI](mutation-cli-red.log), verktyg tools/recall_all_mutations.py OUT. |
| Semantisk | Samma singel-POST/empty body, en kurir per enhet; alla försök trots avslag. Tester: flera/en avvisad/transportfel/ingen marscherande/escaping; CLI alias/text/JSON/ogiltiga kombinationer. Individuell recall bevarad. Null-lista kan inte låtsas lyckad i webb. |
| Användare | Register/join/found/två riktiga marscher→webbknapp respektive kompilerad CLI på var SIN ny PG16/Redis och rena env/tick6. Båda: två accepterade POST202, två olika messenger_ids, två MarchRecalled-audits, båda garnison, noll SQL-mutationer/browserfel. [Webb](live-web/proof.json), [CLI](live-cli/proof.json). CLI-repetitionen namnger båda pending-avslagen, ingen ny order: [text](live-cli/repeat-cli.txt). |
| Visuell | EN knapp + EN kort summa, inga fält/rapportsektioner. Befintliga CSS-klasser, inga nya färger/inline-stilar. [Desktop](live-web/recall-desktop.png), [390px](live-web/recall-mobile.png) vid1:1; mobil-overflow assertion grön. **BILD väntar Timothy i senare bunt**, ej godkänd idag. |
| Paritet | Verblisterad kirurgiskt: Recall all = vanlig recall per marscherande enhet, klientloop. Verb-parity före/efter: inga nya luckor; unresolved0→0, registered-not-listed42→42, client-not-registered2→2, gamla stale batchraden1→0. Snapshots här; inga nya endpointundantag. |
| Drift | Ingen merge/push/deploy av Codex; Claude äger integration efter ordinarie grind. |

## METODISK LÄRDOM OCH AVGRÄNSNING

- Återanvänd spelarens fungerande singelväg; klientloopen ger gemensamt klick
  utan ny servermekanik. Ett avslag får inte stoppa återstående försök.
- Garnison ensam bevisar inte recall: expeditionen kan återvända av sig själv.
  Varje faktiskt orderaudit och separat fysisk kurir verifieras också.

Summan gäller accepterade order, ingen omedelbar hemkomst utlovas. Senare
leveransavslag går via befintliga Dispatches. Ingen ny kanon eller balans.

## Resume checkpoint

Gren codex/recall-all-enkel, /tmp/megaron-codex-recall-all-enkel-20261007,
bas masterb353d54f, kontraktd48a093f, kod7f30b7e5. Sluthash i chat/board.
Bygg OUT/temenos med -X main.buildCommit=7f30b7e5 och OUT/keryx, kör
python3 tools/recall_all_live.py OUT 7f30b7e5 web respektive cli i separata
utdatamappar/färska riggar. Docker/Python Playwright/Chromium krävs.
Verktyget rensar bara egna processer/containers/privatconfig med rm-fv.
Inga nödvändiga verktyg eller proof bor bara i /tmp. Backenddiff mot master
(api/internal/cmd-server) tom; parkerade aggregat-/recall-batch-grenar orörda.
Nästa Claude: granska/integrera och ordna BILD-bunten. Codex fortsätter köad
order H–K först efter denna överlämning.
