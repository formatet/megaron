# Steg 4 — mätt arkitektursvep

Kontrakt före mätning, 2026-10-08. Bas f586270d40840fc8afb550bca80a2ee73d4e7a97.

Problem: prioriteringen av ägarslicar saknar uppmätta kopietal, churn och konsumentmängd. Leverans: dupl Go, jscpd JS, deadcode utan/med test, SQL-fragment, DB-call-sites, 60-dagars git-churn, literaler och produktion/test-kopior, klassificerade mot godkänd ägarkarta 1–19.

Invariant/scope: endast mätverktyg och rapport i denna katalog, egen vault-rapport och länkar. Ingen produktionskod, radering av deadcode eller kanonändring. Huvudgrind: bevisar underlag; implementation prioriteras i steg 5 av Claude.

Acceptans: alla begärda instrument ger rådata och reproducerbara parametrar; varje fynd har ägarrad eller explicit ingen rad; fem kandidater får separat kopie-/churn-/konsumentmått; begränsningar redovisas. Stopvillkor: analysfel eller lucka får inte döljas som noll träffar. Ingen implementation.

Bevis: verktygsversioner, källa låst till bascommit, process-exitkoder, fil/rad-klassificering, riktade kontrollfixturer för eget mätverktyg. Full speltestsvit/mutation av produktionskod inte tillämplig; mätverktyget ska däremot falsifieras.

Arbetsstatus: WIP PARKERAD 2026-10-08 på Claudes order efter OOM. Rådata är insamlad men analys/klassificering och rapport är inte färdiga; inga slutresultat hävdas.
