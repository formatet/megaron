# Första bygget: "— nobody makes stone" (LIVE 812498a, 2026-11-21)

Order: `.agents/order-forsta-bygget-20261118.md`. BILD tagen efter merge, i Firefox 1:1, 1280 och 390 breda,
i acceptansvärlden på `812498a` (mig 258). Nygrundad stad Hagia Triada med 0 sten, 0 timmer och 0 ceder. Skript: `bild.py`.

- `a-buildings-*.png`: Buildings-fliken. Alla rader slutar "— nobody makes stone/timber".
- `b-hexrow-*.png`: byggraden efter klick på en hex (Farm), samma svans.

Fråga till Timothy (BILD): svansen upprepas på varje rad, så listan blir lång på telefon. Behåll, eller visa den en gång ovanför listan?
Utan svans (känd lucka): hexrader i väljläget efter "Choose a hex →" (`city.js:257` har inga rates).
Prov: rött före/grönt efter (Sonnet), mutanter `<=0` och cedergrenen dödade, webb 1050/0, Go-svit grön.

## En gång ovanför listan (LIVE 3355da2, 2026-11-22)
Timothy 11-22: svansen ska stå en gång ovanför listan och inte på varje rad. `c-once-buildings-*.png` visar "Nobody here makes stone or timber." överst i Buildings.
Raderna säger bara "needs 4 stone, city has 0". Hexvägen (`d-once-hexrow-*.png`) lägger samma rad överst. Bilderna a/b ovan visar läget före.
Prov: rött före/grönt efter, mutanten "en rad per byggrad" dödad (Sonnet), helsvit rc=0 (Sonnet), Firefox 1:1 i acceptansvärlden (webben på 3355da2, Go-binären på 75dc435; däremellan ändrades bara webben).
