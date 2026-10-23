# Matpanel — webbläsarparitet 2026-10-23

Acceptansprocess `/healthz`: `{"commit":"a05cddc+dirty","migration":205,"status":"ok"}`. Befintliga isolerade konton BrowserCity23 och BrowserHost23; inga DB-ingrepp.

Problem: matpanelkontrollen väntade ovillkorligt på `.harvest-blessing` och avbröt i en vanlig stad utan aktiv välsignelse. Spelkoden fungerade; verktygets förutsättning var fel.

Ändring: öppna och mät `#city-food-sec`, kräv en icke-tom matsammanfattning och jämför eventuell välsignelse med API-tillståndet. Ingen spelregel eller produktionsyta ändrad. Scope är verktyget och bevisen. Invarianten är att frånvaro av välsignelse är ett giltigt tillstånd. Arbetet bevisar en befintlig spelaryta mot kedjegrinden.

Baslinje: originalverktyget avbröt med Playwright TimeoutError efter 10000 ms väntan på `.harvest-blessing`. Korrigerat verktyg: 18 mätningar, 0 fynd, Firefox/Chromium/WebKit vid 1280 och 390 px. Matpanel + Host/Details. `measurements.json`, `findings.txt` och bilder ligger här.

Slutlig kontroll skärper API-villkoret till samma aktiva tillstånd som renderaren (positiv återstående tid, positiv bonus, minst en vara); denna precisering tillkom efter webbläsarstarten. Det uppmätta kontot saknar välsignelse, så dess gren är oförändrad. Aktiv välsignelse har inte provats i denna körning. Full Go-svit behövs inte för denna verktygsändring. Ingen deploy behövs.
