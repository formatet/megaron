# M — Diplomacy, BILD

## Slice-kontrakt

Problem: fyra flikar delar samma diplomatiska ingång och första brevet kräver Compose.
Spelarsanning: Correspondence för brev/erbjudanden/svar, Known för kända härskare och städer samt Write till samma tidigare kontaktbara destinationer.
Invariant: inga förmågor eller data/FOW-gates ändras; inga rumour-only-destinationer öppnas för dispatch.
Scope: diplomacy.js yta/navigation, main.js bindings, Codex och bevis.
Non-scope: gossip.js, trade-form/body/acceptance logic, server/API/keryx, regler och CSS.
Acceptans: exakt två flikar; kända städer och härskare läsbara med tal i ord; första brev via Known, fortsättning/handelsförslag i tråden, Inspect och Reply bevarade; fel/rumour ger ingen ny dispatch; riktiga brev+köp/säljförslag+svar före/efter.
Stopvillkor: informationsgating/serverfel eller förmåga som inte går att bevara inom ytan rapporteras till Claude.
Bevis: baslinje6d8d5f60 före produktion, faktiska drawer-handler JS regressionsrött, mutation, full JS/fresh Go/vet; var sin fresh PG16/Redis med vanlig register/join/settle/march/browser och healthz, desktop1280 och390 BILD, två rena efterarmar.
Gate: bevisar nåbar brev-/handelsyta för kedjegrinden, inte en full bronskedja.
