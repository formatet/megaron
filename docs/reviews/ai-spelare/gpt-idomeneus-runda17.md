# GPT Idomeneus — runda 17

- **Dag före/efter:** 674 → 674 (inga ticks passerade i de observerade anropen).
- **Verkliga CLI-anrop:** 7/40. Första anropet var status; inget login eller väntan.
- **Läge och plan:** MalliaWest hade 57 silver, 97 grain vid första statusen, positiv matbalans och en galär i garnison. Enhetslistan bekräftade att inga andra expeditioner pågick; galären `Palace Herald` (ID `dd6e8ea0-7aa7-4c26-835d-6d72bebd4547`) var tillgänglig. Målet var att utforska kring kopparmarkören (9,64) och säkra bronstillgången. Codex anger att en galär-expedition laddar proviant före avfärd och utforskar självständigt; utfallet är inte känt förrän rapporten kommer hem.

**Orderhändelser i denna runda (råloggspostnummer):**

- Post 2: avvisat listanrop `unit list --province MalliaWest`, exakt fel: `Error: unknown flag: --province`. Det skickade ingen spelorder.
- Post 5: preview för `unit march --unit dd6e8ea0-7aa7-4c26-835d-6d72bebd4547 --target 9,64 --intent explore --ticks 30 --preview`. Resultat: `Planned expedition: 30 days — with immediate departure, home by day 704. First-leg arrival remains unknown.` samt `treasury lasts 37.8 days` och `No order sent.`
- Post 6: accepterad order, samma march utan `--preview`: `Unit dd6e8ea0-7aa7-4c26-835d-6d72bebd4547 exploring (9,64) — arrives in 9 days — Expedition around (9,64), 30 days; turns home by day 689; home by day 704`.

Efter ordern bekräftade post 7 fortsatt dag 674, MalliaWest 57 silver och 95 grain, med arméns silvernetto −1.5/dag. Expeditionsutfallet och kopparfynd är ännu obekräftade.

**A/B/C-fynd från aktuell rålogg:**

- **A — går bet/yta saknas:** inget aktuellt hinder på ordern. Förhandsvisningen varnade att ankomst/sträcka är okänd eftersom rutten går över okänd terräng, men expeditionen accepterades.
- **B — återgivet valideringsfel, ingen belagd felaktig text:** post 2: `unit list --province MalliaWest` gav `Error: unknown flag: --province`. Hjälpen i post 3 visar att `unit list` saknar sådan flagga; post 4:s vanliga `unit list --json` fungerade.
- **C — småsaker:** post 7 varnade `Timber production ~0` och angav att timmer kräver placering av arbetare på timmerhex. Inga bygg- eller arbetsorder skickades.

Förälderns semantiska kontroll: att en flagga som inte finns i hjälpen avvisas visar ingen motsägelse eller ny produktbugg. --province-avslaget är ett CLI-inmatningsfel; rättad vanlig unit-listning fungerade.

Föräldrakontroll: 7 råposter, obruten låst numrering 1–7, budget ≤40 och status dag674 före/efter verifierade. Accepterad marsch, preview och CLI-avslag räknas separat. Inga HTTP500, timeout eller vänteslingor. Verktygskällan i `runda16-metod.md` är oförändrad utom loggsuffixet `runda16-tools` → `runda17-tools`; aktuell SHA-256 `4c67a400583186fdd82a72f9d4a55e4bd6e09a654f0f01762629397682277463`.
