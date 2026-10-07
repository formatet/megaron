# Paritetsrapport fyra ytor

## Kontrakt före kod
Problem: manuella verblistan saknar reproducerbar kodkorsning.
Spelarsanning: rapport synliggör befintliga ytluckor, inga verb ändras.
Invariant: bara läsning; metod + rutt och källevidens, dynamiska oklarheter synliga.
Scope: tools/verb_parity.py, explicit allowlist/aliases, parserfixturtester, verktygsindex och rapport.
Non-scope: produktionskod, DB, ändring av verblistan eller auto-fix av luckor.
Acceptans: chi-nästning, CLI-format/variabler, webbkonkat/template/metod,
Codex-omnämnanden, allowlist och verblistadiff kan återges med parserprov;
första körning med rangordnade, manuellt kontrollerade fynd.
Stopvillkor: spelkanon/produktionsförändring krävs, då bara fynd till Claude.
Bevis: unittest-fixturer, fysisk parsermutation, riktig körning och källkorsning.
Kedjegrind: bevisar att handlingar har nåbara klientytor; statisk rapport
bevisar inte runtime, upptäckbarhet eller att artikeltexten är begriplig.

## Resultat
PENDING
