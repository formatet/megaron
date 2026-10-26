# Täckning och mutationer — marschpanel

Mutanter körda 2026-10-26 i separat clean worktree `/tmp/megaron-marschpanel-mutants-final-20261026`, källa `764b0bc5`. Ingen produktionsfil ändrades i bild-/huvudworktree. Varje mutation återställdes till exakt ursprungliga bytes; SHA256 finns i [maskinmanifestet](mutations/verified.json). Go kördes alltid genom `tools/gotest.sh`, egen färsk PostgreSQL migration215, ren testmiljö och `-p1`. Alla tio nedanstående blev assertion-röda, ingen compilefail räknad. Återställd kod körs grönt i `mutations/restored-green.log` och `mutations/restored-js-green.log`.

## Nya eller utökade prov

| Prov | Täcker | Täcker inte |
|---|---|---|
| `TestMarchPreview_ReadOnlyAndTiming` | Riktigt HTTP/DB: garrisonkort budget GET+POSTparitet; positionedkort faktisk retur nekas före Runner; tillräcklig positionedbudget200 utan påstådd avgång; båda avslagen maskerar dold minimitid; snapshot utan writes | alla terräng-/skeppskombinationer; isolerad första halvtidsgrind när returgrinden också skulle neka; faktisk budleverans efter order |
| `TestResolveUnitExactDisplayName` | Exakt case-/blankstegsnormaliserat namn, unikt ID, tvetydigt namn nekas | Varje kommando som använder gemensam resolver; IDprefix täcks äldre prov |
| `TestExploreRefusalBeforeRunnerDispatch` | Keryx CLI med HTTPtestserver:422forecast ger1GET/0POST | Riktig server/DB i detta enhetsprov; nätfel, samtliga kommandoflaggor |
| `TestExploreTicksOrderAndPreview` | march/alias, dispatch GET+POST och preview endastGET, mål/intent/ticks vidarebefordras | Faktisk serverfysik, alla valfria parametrar |
| `H: defaults select one…` + namedchoiceprov | Högst1default, separat checkbox/ID för tredje kohorten, sharedselection endastvaltID i preview+POST | Riktig DOMlayout; den verifieras i nativeBILD |
| `too-short expedition…` |422 ger0POST även positioned Runner; samma IDs och budget till preflight | Servermekanik, alla transportfel |
| `closing the menu…` och `changing the named selection…` | Stäng/ändrat val under pendingpreflight ger0stalePOST; freshsend återaktiverad och bara nyaID | Världsbyte testas inte separat; samma produktionsguard omfattar även world/destination |

## Exakta egna mutanter

Kommandona använder GNU sed och körs i separat clean checkout. Återställ med `git -C "$CHECKOUT" restore -- <fil>` efter VARJE rad. Det reproducerbara skriptet `tools/marschpanel_mutants_20261026.py "$CHECKOUT"` använder samma exakta substitutioner, bevarar bytes och avbryter om en källa inte matchar exakt en gång.

### web-default — assertion-röd

```sh
sed -z -i 's#index === 0 && group\.ids\.length ? 1 : 0#index === 0 \&\& group.ids.length ? group.ids.length : 0#' web/static/js/megaron/ui/marchctx.js
```

Förväntat rött: `H: defaults select one`. Faktiskt utfall: [rålogg](mutations/web-default.log).

### web-selection — assertion-röd

```sh
sed -z -i 's#if(selected(i,k))picks\.push#if(selected(i,0))picks.push#' web/static/js/megaron/ui/marchctx.js
```

Förväntat rött: `named choices let the third`. Faktiskt utfall: [rålogg](mutations/web-selection.log).

### web-preflight — assertion-röd

```sh
sed -z -i 's#p\.order\.intent==='"'"'explore'"'"'&&p\.mode!=='"'"'redirect'"'"'#false\&\&p.order.intent==='"'"'explore'"'"'\&\&p.mode!=='"'"'redirect'"'"'#' web/static/js/megaron/ui/marchctx.js
```

Förväntat rött: `too-short expedition is refused`. Faktiskt utfall: [rålogg](mutations/web-preflight.log).

### web-version — assertion-röd

```sh
sed -z -i 's#selectionVersion!==marchSelectionVersion||world!==State\.WORLD_ID||destination!==State\.marchCtxDest#world!==State.WORLD_ID||destination!==State.marchCtxDest#' web/static/js/megaron/ui/marchctx.js
```

Förväntat rött: `changing the named selection`. Faktiskt utfall: [rålogg](mutations/web-version.log).

### keryx-name — assertion-röd

```sh
sed -z -i 's#strings\.EqualFold(strings\.TrimSpace(u\.DisplayName), needle) || ##' server/cmd/keryx/cmd_resolve.go
```

Förväntat rött: `TestResolveUnitExactDisplayName`. Faktiskt utfall: [rålogg](mutations/keryx-name.log).

### keryx-preflight — assertion-röd

```sh
sed -z -i 's#if intent == "explore" {\n				if _, err := fetchMarchPreview#if false \&\& intent == "explore" {\n				if _, err := fetchMarchPreview#' server/cmd/keryx/cmd_unit.go
```

Förväntat rött: `TestExploreRefusalBeforeRunnerDispatch`. Faktiskt utfall: [rålogg](mutations/keryx-preflight.log).

### keryx-query — assertion-röd

```sh
sed -z -i 's#query\.Set(key, fmt\.Sprint(value))#query.Set(key, fmt.Sprint(value)+"0")#' server/cmd/keryx/cmd_march_preview.go
```

Förväntat rött: `TestExploreTicksOrderAndPreview`. Faktiskt utfall: [rålogg](mutations/keryx-query.log).

### api-mask — assertion-röd

```sh
sed -z -i 's#if rej\.Code == combat\.RejectExpeditionBudget {#if false \&\& rej.Code == combat.RejectExpeditionBudget {#' server/api/handlers/unit_march_preview.go
```

Förväntat rött: `TestMarchPreview_ReadOnlyAndTiming`. Faktiskt utfall: [rålogg](mutations/api-mask.log).

### return-budget — assertion-röd

```sh
sed -z -i 's#if travelTicks+returnTicks > expeditionPlan\.LengthTicks {#if false \&\& travelTicks+returnTicks > expeditionPlan.LengthTicks {#' server/internal/combat/march_start.go
```

Förväntat rött: `TestMarchPreview_ReadOnlyAndTiming`. Faktiskt utfall: [rålogg](mutations/return-budget.log).

### outbound-code — assertion-röd

```sh
sed -z -i 's#return nil, rejectExpeditionBudget("the chosen hex#return nil, reject(http.StatusUnprocessableEntity, "the chosen hex#' server/internal/combat/march_start.go
```

Förväntat rött: `TestMarchPreview_ReadOnlyAndTiming`. Faktiskt utfall: [rålogg](mutations/outbound-code.log).

## Explorativ överlevare — tydlig avgränsning

`march_start.go`: `if currentTick+travelTicks > expeditionPlan.TurnTick {` → `if false && currentTick+travelTicks > expeditionPlan.TurnTick {` överlevde APIprovet ([logg](mutations/outbound-budget.log), [manifest](mutations/exploratory.json)). För garrisonens symmetriska rutt nekar den efterföljande `travelTicks+returnTicks > LengthTicks` fortfarande samma expedition. Det bevisar ingen separat first-leg-gräns. Slutmatrisens `outbound-code` tar i stället bort den NYA budgettaggen enbart från första avslaget och blir röd på garrisonens sekretessassertion. Returgrinden är separat assertion-röd genom positionedfallet. Ingen ny budgetmekanik byggdes i denna slice.
