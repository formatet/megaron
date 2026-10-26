# Täckning och mutationer

Slutkärna `1feeec6d`, migration217. Fyra assertion-röda mutanter i separat clean checkout, egen tom DB för varje arm via gotest.sh, byteexakt restore och nya riktade gröna prov. Ingen compilefail räknas. SHA256 och faktiskt källa i mutations/verified.json.

| Prov | Täcker | Täcker inte |
|---|---|---|
| TestRedirectExpeditionReturnArrivesAtActualDestination | verklig expedition→senareben→recall→hemresa→redirect; släckt returnintent/hemankare; tomhexpositioned, hemhexgarrison, oförändrad support/koordinat, gammaltarrivaljobb/idempotens, ingen falsk rapport | utåtgående explorelegacy-beteende (oförändrat), alla skeppsuppdrag, alla terränger |
| TestRemoteLandGarrisonCorrectionPreservesPhysicalState | verklig217SQL och dubbelkörning, felaktiglandgarrison→positioned, exaktq/r/ägare/support; korrektland, navalhamn och okändq/r bevaras; audit exakt1 | varje historiskdataskada; inga enheter med NULLq/r får påhittad plats |
| Äldre TestRecallExpedition* / TestRecallPlainAndExpeditionRedirectDestinations | land/navalrecall till hemstad/report, ägarförlust, minne, utåtgående redirectlegacy kvar | samtliga andra orderverb |
| return_redirect_upgrade.py | read-only driftbackup→egenPG16,216→217→216→217,5thenda korrigering, fysiska enhetsfält/payor/resurser/konton/värld/memoryhashar bevarade, gamlaeventprefix byteoförändrat, exakt1nyauditutfall efter omkörning | skriver inget till drift, backup publiceras inte; rollback återinför avsiktligt inte buggen |
| return_redirect_acceptance.py | egenPG/Redis/server, uttrycklig inkommande returruttscen, riktigKeryx→HTTP202→Runner→worker→fältankomst, fördröjtgammaltjobb verkningslöst, APIpositioned och nativeFirefox1280/390 enhetskort | första returrutten är scenfixtur, inte skapad genom mänskligt playtest; budförföljning ligger i separat WIP |

Ingen ny spelhandling införs: serverns gemensamma redirect/arrival korrigeras, befintliga webb/Keryx/Codex-ytor använder samma sanning. BILD innehåller faktisk fältenhet, ingen renderingsändring eller ny statustext.

## Exakta GNU sed-rader

Kör på separat clean checkout och återställ källan mellan varje rad. Kommandona har byteverifierats mot de faktiskt körda mutationerna.

### return-intent

```sh
sed -z -E -i 's#if\ o\.Mode\ ==\ "redirect"\ \&\&\ marchIntent\ !=\ nil\ \&\&\ \*marchIntent\ ==\ "explore_return"\ \{#if false \&\& o.Mode == "redirect" \&\& marchIntent != nil \&\& *marchIntent == "explore_return" {#1' server/internal/combat/recall_redirect.go
```

Assertion-rött: `TestRedirectExpeditionReturnArrivesAtActualDestination`. [Logg](mutations/return-intent.log).

### home-anchor

```sh
sed -z -E -i 's#newIntent,\ newColonyName,\ homeSettlementID\ =\ nil,\ nil,\ nil#newIntent, newColonyName, homeSettlementID = nil, nil, homeSettlementID#1' server/internal/combat/recall_redirect.go
```

Assertion-rött: `TestRedirectExpeditionReturnArrivesAtActualDestination`. [Logg](mutations/home-anchor.log).

### naval-exemption

```sh
sed -z -E -i 's#u\.category='"'"'land'"'"'\ AND\ ##1' server/db/migrations/217_correct_remote_land_garrisons.up.sql
```

Assertion-rött: `TestRemoteLandGarrisonCorrectionPreservesPhysicalState`. [Logg](mutations/naval-exemption.log).

### correction-audit

```sh
sed -z -E -i 's#UnitGarrisonLocationCorrectedV1#UnitGarrisonLocationWrongV1#1' server/db/migrations/217_correct_remote_land_garrisons.up.sql
```

Assertion-rött: `TestRemoteLandGarrisonCorrectionPreservesPhysicalState`. [Logg](mutations/correction-audit.log).
