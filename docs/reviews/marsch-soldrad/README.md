# Marschens soldrad

## Slice-kontrakt

Problem: marschens preview visar ankomst men inte hur solden ändras.
Spelarsanning: samma enda soldrad i webb och Keryx visar urvalets aktuella
silveravdrag, avdrag i fält och betalarstadens soldreserv i ticks (days på ytan).
Invariant: en preview är read-only, använder samma upkeep/cirkulation som tick,
och avslöjar aldrig någon annans enheter eller kassa.
Scope: server-preview, delad soldcirkulation, CLI/webb/Codex och prov.
Non-scope: nya soldnivåer, balansändringar, orderregler, migrations- eller auditkod.
Acceptans: faktisk tick jämförs, spjut 0→0, nollavdrag ger null,
färsk Go/JS/vet och Firefox 1280/390 innan integration.
Stopvillkor: kanonkonflikt parkeras i vault BESLUT; oberoende köarbete fortsätter.
Bevisplan: auth-HTTP/PG, tickdebitering, fysisk mutation byteåterställd,
två rena native körningar och healthz efter deploy.

Baslinje: master eb6d57c1, migration 178. Färsk
TestMarchPreview_ReadOnlyAndTiming grön före serverändringar. Klientändringar
fanns då redan i den egna worktreen, men påverkar inte HTTP-baslinjen.

## Implementationsval

Webburvalet kan omfatta flera betalande städer. Alla preview-GET får samma
unit_ids; enhets-ID och medföljande skeppslast dedupliceras. Svarens soldtal
avser hela urvalet, och webb visar en enda gemensam soldrad.
För flera kassor visas den kortaste runway (silver_payer_count märker texten),
utan att summera pengar som inte kan flyttas omedelbart mellan städer.
Runway använder stadens totala nettosold efter avmarsch, även ej valda enheter,
inklusive reparerande/fraktande skepp. Den är en soldreserv, inte en prognos
för framtida produktion/handel/rekrytering. Mine income subtraheras därför inte
från orderns uttryckliga soldavdrag. Saknad betalare för soldkrävande enhet ger
runway 0; gratis spjut och aktiv founder phase ger inget soldavdrag.
Redan marscherande enheter visar fortsatt fältsold medan ankomst förblir okänd.

Status: implementation och verifiering pågår; ej mergad eller deployad.
