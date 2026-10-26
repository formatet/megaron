# Native Firefox acceptans

Kör `python3 tools/marschpanel_bild_20261026.py docs/reviews/marschpanel/bild` på stabil kod. Riggen bygger sin egen Temenos/Keryx, startar egna Pg16/Redis7-containrar och stänger allt i finally. `proof.json` anger processens healthz och hashmanifest; `fixture.sql` visar samtliga explicita scenförutsättningar.

M1 visar högst en förvald kohort. M2 väljer enbart tredje namngivna kohorten. M3 följer ett riktigt browser-POST med202; DB har exakt en ny Runner med just dess unit_id. M4 skickar en expedition på6 game days mot ett långt okänt mål: verklig422-preview ger Nothing sent,0nyaPOST och oförändradeorderbud. Alla fyra steg körs i native Firefox1280×900 och390×844. Bilderna är ögonlästa vid1:1.

Avgränsningar: register/join/founding sker genomAPI; platt terräng, stora lager och tre positionedkohorter per viewport är uttryckliga SQL-scenfixturer. Appens riktiga march-controller öppnar målkoordinaten; inget API-svar eller DOM-innehåll ersätts. Ingen budankomst eller slutförd marsch påstås. Verklig Keryxbinary använder privat0600tempconfig mot samma riktiga server: exakt namn med --preview lyckas utan nytt bud; en vanlig dispatchvariant med --intent explore --ticks6 avvisas med exit1/HTTP422 före budskrivning. Se keryx-preview-*.json och keryx-short-*.json. Inget lyckat CLI-dispatchförlopp påstås. Mobilens panel är appens befintliga helskärmsvy.

Inledande två riggförsök stoppade före acceptans vid en obefintlig hårdkodad foundingkoordinat; katalogval av ett befintligt hex ersatte antagandet. Samtliga försök städade sina egna processer och containrar. Ingen produktfix härrör från dessa fixturfel.

Första rena bildkörningen hittade två layout-/statusfel: lång avslagstext pressades bredvidknappen och avbrutenETA lämnade Estimating arrival. Dessa rättades i fb36d1d9. Alla slutbilder och proof.json är från en HELT ny isolerad körning därefter; åtta bilder omgranskade1:1 visar feltext på egen rad och ingen kvarhängandeETA. Äldre bilder har ersatts.

Slutkörningen inklusive nativeKeryx byggdes från healthz0b12fb89+marschpanel/migration215. BådaCLI-scenarier kördes för vardera viewportens explicita3kohortsuppställning och jämförde helaorderbudsnapshoten före/efter. Alla8slutbilder omgranskade efter denna körning.
