# Marschpanelen: välj enheter vid namn

Kontrakt före kod2026-10-26.
Problem: standardantalet omfattar hela första gruppen och val av en annan namngiven kohort är omöjligt; förhandsvisningsfel hindrar inte sändning av för kort expedition.
Spelarsanning: se och välj exakt namngivna enheter före sändning; en för kort expedition lämnar inget bud.
Invariant: enbart valda ID får order; blandad march/redirect behåller sin fysiska Runner-väg. Förhandsvisning får inte avslöja dold rutt/restid. Domänens befintliga tur/returregel ändras inte.
Scope: webbens enhetsval+förhandsvisning+sändning, befintlig serverbudget och dess prov, Keryx explicit ID/preview, Codex marching.
Avgränsning: bud som följer en flyttad enhet är nästa slice; inget annat budbeteende eller balans ändras.
Acceptans: högst en förvald enhet; val av annan namngiven enhet; samma IDs preview/send; för kort budget stoppar skickandet; namn syns före sändning på fyra ytor.
Stopvillkor: ny informations-/mekanikkonflikt tas direkt till Timothy.
Bevis: baseline-röda prov, verkliga API-budgetprov, selektions- och asyncprov, mutanter, färsk fullGo/JS, nativeFirefox1280/390 och livehash/grundtillstånd.
Gate: proves läsbar ordergivning i kedjans geografisteg.

## Checkpoint inför clear

Byggt men INTE integrerat/deployat: individuella checkboxar med namn, högst1förvald; preview och send använder samma valdaID; expeditioners read-only budgetkontroll måste lyckas före någonPOST; stängd/ändrad panel invaliderar väntande sändning. Keryx accepterar exakt visningsnamn och gör samma preflight före expedition. Codex uppdaterad.

Servern nekade redan för kort expedition för både garrison och positioned. Nytt riktigt positionedAPI-prov hittade i stället ett informationsläckage: avslag angav dold minsta restid. Additiv OrderReject.Code markerar de två budgetavslagen; ENDAST preview maskerar dem. Gammal POST-/eventbetydelse och mekanik bevaras.

Gröna delprov:54JS i marchctx+march_preview; helaKeryx-paketet på färskDB; serverns TestMarchPreview_ReadOnlyAndTiming på färskDB; domänens asymmetrisk retur/underbemannat skepp. Baseline-röda enhetsval/budget/Keryx och sekretess finns bredvid. Domänens returgränsmutant var assertionsröd på separat checkout; övriga mutanter återstår.

Kvar före integration: granska slutdiff; bredda meningsfulla klientprov för ändrat val under preflight om nödvändigt; fysiska mutanter och täckningsgränser per kärnfil; nativeFirefox1280/390 med RIKTIGA API-data (välj annan namngiven kohort och visa att bara den får order, samt för kort expedition ger0bud). Därefter fetch+rebase origin/master STRAX före hash, full färskGo/helaJS/vet/4dayguards på slutkoden, FFmerge/push/deploy/livehash+grundtillstånd/assetverifiering, todo/vault/board. Inga nya kanonbeslut behövs för nuvarande kontrakt.
