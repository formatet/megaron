# ORDER 16 — inventering och live-lönejämförelse före kod

Endast dokument och read-only SELECT. Gren codex/sold, bas ea97b332.
Live healthz ea97b33/mig198, snapshot2026-10-22 17:17:14UTC. Databasen bekräftar
transaction_read_only=on; capture.py använder tools/alfa_db.Db.
Runtime poleia-server pid52285 har ingen UPKEEP_SOLD_SHARE: kodens default0.7 gäller.

Se [comparison.md](comparison.md) för varje enhet och före/efter;
[consumer-inventory.md](consumer-inventory.md) för konsumenter och filkrockar.
Snapshot/rateberäkningen kan upprepas med capture.py och compare.py. Kontrollera
runtime-share igen före ny jämförelse; compare.py läser endast sparade filer.

16 befintliga enheter:12spearman,1nomadic_host och3skepp varav1forming.
Två aktiva avlönade skepp, båda garrison med betalande stad:
full galley Crew20 och merchantman Crew10 (se radtabell). Forminggalären har
ingen löpande debitering än; dess framtida prognos måste följa samma nya ägare.
Spearman förblir0silver. Ingen elite/chariot/war_galley i snapshoten; deras
faktorer är beslutad regel och behöver syntetiska riktiga DB-tester före implementationens handover.

Ny lönegrund1silver/100man och tick i fält, halv hemma; elitefaktor1.5,
chariot2, naval faktiskCrew/100. 70%-krediten tas bort. Mat/VoyageRation
behåller ORDER12-regeln, silver är separat. Laid_up0. En ägare i UnitUpkeep,
inte kopierade silverkolumnvärden. Aktuell nettolön är en behovsprognos;
vi har inte dragit något, poolat kassor eller påstått att alla kan betala.

Kritiska kodställen före genomförande: unit_march_silver.go SELECT/ScanCrew
istället0, separat nuvarande/fältstatus; UpkeepSettled historisk semantik
bevaras med circulation0/{} efter avvecklingen, gross=destroyed, unpaid separat;
SilverAuditV2 stockmått kvar. Unpaid72ticks/desertion10man orörda.

Filkrockar: province.go delas med13/17; klient/Codex med13/14/17. Denna gren
är endast inventering påea97b332 och ska utgå från integrerad master före kod.
BILD om faktiskt webbutseende ändras; riktade DB/röda basprov/fysiskmutation,
JS/vet och full fresh EN gång för kodslicen. Inga tester/mutationer körs eller
påstås i denna docs-only inventering. Ingen produktionskod/migration/livedata ändrad.

STOPP: Claude ska granska denna konkreta jämförelse och ge kod-ok.
