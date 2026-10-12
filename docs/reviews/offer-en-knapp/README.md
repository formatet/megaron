# Offer — en knapp per bön

Inventering före kod på **53d33910**, gren `codex/offer-en-knapp`, worktree `/home/tk/wt/codex-offer-en-knapp`. Claude integrerar; hash + STOPP.

| Kontrakt | Innehåll |
| --- | --- |
| Problem | Composer lägger ett varufält per tillgänglig vara under varje bön. |
| Spelarsanning | En Offer-knapp per befintlig bön; servern väljer samma traditionella recept som tidigare. |
| Invariant | Ingen bön, serverregel, kostnad eller odds ändras; compose finns inte i webbens DOM. |
| Scope | Kult-rendering, bortkopplade composerhandlers/extra goodsread, Codex, prov/rigg. |
| Non-scope | Server/Keryxmekanik, ny bönemodell, generellt drawer-polish. |
| Acceptans | Alla befintliga böner kvar, samma POSTbody utan offering; DOM ej composer + fysisk mutation; JS gröna; Firefox1280+390; inlinefeedback. |
| Stopvillkor | Compose ensam väg till bön → STOPP. Inventeringen visar ingen sådan bön. |
| Bevis | Baslinje först, inventering av faktisk serverdefault/CLI, native DOM/mutation, normalt Offer-flöde i rigg. |

Kedjegrind: bevisar begriplig och nåbar tempelhandling, inte en ändrad ekonomisk/bönemekanik.

## Inventering före implementation

| Väg | Request och grind | Slutsats |
| --- | --- | --- |
| Webb Perform → okRite → castRite | POST `/worlds/{w}/settlements/{s}/rite`, `{prayer:id}`, ingen offering. Knapp om !cooldown och affordable och ingen aktiv frenzy. | Befintlig traditionell väg för varje visad bön. Rename Offer, samma body. |
| Webb composer → okRiteComposed → castRite | Samma endpoint/prayer, tillägg `{offering:{good:qty}}`; tom composer faller tillbaka till okRite. Composer bara när !cooldown och affordable. | Inte ensam väg till någon bön; tas bort från webb. Aktiv frenzy har ingen Performknapp men är redan aktiv, inte en exklusiv bön. |
| Server SettlementHandler.Rite | settlement.go998 använder body.Prayer, bara tomt id får defaultBattleFrenzy. 1122: utan composed offering används scaleOffering(spec.Offering, default multiplier1); 1125: composed använder explicit body.Offering. | Samma bön och recept nås utan compose. Regler orörda. |
| Keryx rite | cmd_rite.go204 explicit --prayer; utan --offer inget offeringfält; 213–222 explicit --offer -> body.offering. --offer-multiplier också kvar. | Båda vägar finns; AI-yta behåller --offer. Inget dubbelt kommando, ingen CLIändring. |
| Katalog | religion.PrayerSpecs och CulturePrayers: tre böner per kultur, alla har traditionellt Offering; province.go536 affordability läser tempel/kharis/receptstock. | Samma val, gates och serverkatalog kvar. |
| Inlinefeedback | kult.js använder redan showInlineResult efter rite, inga alert-träffar. | Orderns fyra alert är en stale basuppgift; tidigare slice hade redan konverterat dem. Bevara och prova inline. |

**Claude 2026-10-12 11:42:** tolkningen godkänd: **en Offer per befintlig bön**, inte en gemensam tempelknapp (den skulle kräva ny bönemekanik). Compose helt borta ur webben; Keryx --offer kvar. Ingen STOPP-konflikt.

## Grindar

Pågår. Server och Keryx ändras inte. Rapporten fylls med baslinje, DOM-mutation, aktuella assets och Firefoxbilder före handover.
