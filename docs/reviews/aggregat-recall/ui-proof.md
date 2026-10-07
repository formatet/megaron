# G — Recall-rutan, BILD-underlag

Kontrakt [ui-contract.md](ui-contract.md), innan kod99094e18. Gren
codex/recall-batch-ui från claude/aggregat-recall-merged079fca7d i egen
/tmp/megaron-codex-recall-ui-20261007. UI83814797, masterintegration758982c4
+00b1a662, slutmaster F5605b179. Ingen merge till master/push/deploy av Codex.

Rutan använder enbart befintliga field, btn-small/btn-primary, empty-state,
incoming-header och march-list. Inga nya CSS-klasser, inline-stilar eller färger.
All selected är borta: alla matchande marscherande enheter, eller filter på
befintligt mål/task, budets leverans och gemensam Runner förklarade. Grupper,
egen Runner och varje namngivet avslag återges; passageord står i spelarspråk.
Inga ändringar i body/filter/endpoints eller serverregler genom UI-ändringen.

Masterintegration: fyra konflikter lösta med aggregatets prepareRecall och
pending gruppmanifest, masterroute/FOW/typed OrderReject, aktuella CLI-regler,
båda Codex-proven och båda verktygsrecepten. Ny bare return från master måste
anpassas till prepareRecall-signaturen (*preparedRecall,bool). Riktad fysisk
mutation av borttagen typed OrderReject-mappning gav namngivet assertionrött
HTTP500 i TestRecallNoRoute_HTTPRejectsNamed/course; återställt no-route+batch
på separat färsk DB grönt. UI: befintliga rendering/escaping/body/networkprov
uppdaterade för texten, funktion oförändrad.

385 JS gröna och full env-i vet grönt på slutmaster5605b179. Full Go NY
PG16/migration160 alla paket gröna inklusive world89.083s. Loggar sparas i denna mapp.

Riktig gruppwebbrigg: egna PG16/Redis, rena env/tick6, health5605b179/mig160.
Register→join→found→två riktiga CLI-marscher→War/Army Recall-knapp. Två mottagare
delar bud; två faktiska MarchRecalled-audits och båda till garnison. Namngivet
avslag för upplöst host. Inga browserfel eller SQL-mutationer; SQL-audit läses
endast efter hela resan. [Gruppproof](live-web/proof.json),
[desktop](live-web/recall-desktop.png), [390px](live-web/recall-mobile.png).
Lawagetas-rådet stängdes med sin riktiga Dismiss-knapp och drawern scrollades
till rutan; ingen DOM-markup eller spelläge förfalskades för bilden.


Singelwebbrigg på en ANNAN ny PG16/Redis: en expedition och en vanlig marsch
mot synlig landhex, verkligt task-filter explore→en egen Runner, sedan blankt
recall-all→pending-avslag för första och egen Runner för andra. Slutrapporten
listar Single unit, named pending-avslag och upplöst host-avslag; båda mottagare
har varsin faktisk MarchRecalled-audit och är i garnison. Health5605b179/mig160,
inga browserfel/SQL-mutationer, ingen drawer-overflow vid390px.
[Singelproof](live-web/single-proof.json),
[singeldesktop](live-web/single-recall-desktop.png),
[singelmobil](live-web/single-recall-mobile.png).

Tidiga ej accepterade armar: tredje enheten stoppades vid riktig barracks-gate;
ingen admin/bypass infördes. Browser efter marsch kunde fördröja recall tills
banan fullbordats, därför öppnas browser/drawer före order nu. Två olika mål
bevisar inte skilda kurirmöten (gemensam prefixrutt kan gruppera dem); singel-
scenen använder verkligt task-filter följt av recall-all och pending-avslag.

BILD **väntar Timothy**, inga bilder ögonbedömda/godkända idag. TEXT är
spelarhjälp om det befintliga verbet, inga nya mekanikbeslut. Gamla singelfyndet
i ursprungsrapporten är nu separat rättat/mergat av Claude; det är historik,
ingen kvarstående ny upptäckt i denna UI-slice.

Resume: granska denna diff/bilder och integrera efter ordinarie grind;
Claude äger integration, Timothy bilder. Bygg OUT/temenos med
-X main.buildCommit=5605b179 och OUT/keryx; kör
python3 tools/recall_live.py OUT 5605b179 web för grupp, respektive
python3 tools/recall_live.py OUT 5605b179 web singles i en annan färsk rigg.
Docker/Python Playwright/Chromium krävs; endast egna containers/process/config
rensas och docker rm -fv tar egna anonyma volymer. Funktionella prov och
recept är klara i gren; hash för slutöverlämning står i chat/board.
