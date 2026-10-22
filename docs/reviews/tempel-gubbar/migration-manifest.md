# Torrmanifest före migrationskod

Endast SELECT; ingen livedata ändrad. Snapshot och godkända regel B, inte en utförd migration.

| Stad | Mål | Lediga ordinals | Omplacering (högsta pruningprioritet/ordinal först) |
|---|---:|---|---|
| Vasiliki | 2 | [15, 16] |  |
| Gournia | 2 | [] | 48: bronze foundry, 31: wine (38, 27) |
| Amnissos | 2 | [] | 96: wine (5, 39), 95: wine (5, 39) |
| Damon | 2 | [88, 89] |  |
| Zakros | 2 | [86, 87] |  |
| Lato | 2 | [84, 85] |  |
| Kydonia | 2 | [] | 74: wine (7, 27), 73: oil (5, 28) |
| Petras | 2 | [72, 73] |  |

Deployment kräver nytt manifest: världen ändras. Avbryt om lediga + icke-matjobb inte täcker målet. Inga matjobb eller personer skapas/försvinner.

## Färskt manifest före deploy av mig 201 (2026-10-22 21:03, live d248b09/mig 200, Claude)

Samma SELECT (BEGIN READ ONLY) mot live; målen räknade med transitionens egen regel. Ordinals skiljer sig från första manifestet eftersom fiskeövergången (mig 200) frigjorde platser. Ingen stad avbryter.

| Stad | Mål | Lediga ordinals | Omplacering | Utfall |
|---|---:|---|---|---|
| Vasiliki | 2 | [15, 16] |  | OK | gubbar 23, placerade 14
| Phalasarna | – (L0) | | |
| Gournia | 2 | [] | 48: bronze (foundryNone,None), 31: wine (38,27) | OK | gubbar 66, placerade 66
| Amnissos | 2 | [] | 96: wine (5,39), 95: wine (5,39) | OK | gubbar 112, placerade 112
| Pylos | – (L0) | | |
| Plateia | – (L0) | | |
| Damon | 2 | [25, 33] |  | OK | gubbar 179, placerade 55
| Phaistos | – (L0) | | |
| Zakros | 2 | [3, 4] |  | OK | gubbar 126, placerade 91
| Lato | 2 | [12, 13] |  | OK | gubbar 162, placerade 59
| Kydonia | 2 | [] | 74: wine (7,27), 73: oil (5,28) | OK | gubbar 74, placerade 74
| Petras | 2 | [9, 10] |  | OK | gubbar 145, placerade 47