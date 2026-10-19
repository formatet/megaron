# 21b — stadsmiljö med horisont

## SLICE-KONTRAKT
Problem: Timothy har valt gatubilden04 som huvudspår och tak-/gårdsbilden10, men storlek, mur och tillstånd behöver läsas i dessa kameror.
Spelarsanning: en bebodd stad med horisont; tydlig skillnad mellan liten/stor, mur0/1/3 och normalt/svält/belägring/tempelbygge.
Invariant: samma bronsåldersstad, helt pixelraster, status fristående, ingen ändring i levande stadsvy.
Scope: procedurstudie, PNG, datamatris och galleri i denna katalog, länk från föregående galleri.
Non-scope: live-renderer, API, regler, server och balans.
Acceptans: båda kameror i fyra tillstånd; båda i3×3pop/mur-matris; tre kompositions-/ljusvarianter var inklusive varma berg från03; heltalszoom som ryms i390px.
Stopvillkor: hash+STOPP till Claude; Timothy dömer BILD.
Bevisplan: faktisk160×96-raster med ×2-spelprov och×4PNG, Firefox390/desktop, determinism, lagerkomposition och mutationer som tar bort dataskala eller status.
Kedjegrinden: väntar till efter; förslag till bildkommunikation på befintlig stadsyta.

Referens: Phaistos-lik kuststad,4500invånare, mur1, tempel1, gård2, hamn/varv/gruva. Population/mur varierar endast i den särskilt märkta matrisen.

## SAMMANFATTNING

28 distinkta scener:04+10 i fyra tillstånd, båda kamerorna i nio population/mur-kombinationer, tre bildvarianter per kamera. Alla motiv har160×96 logisk PNG samt exakt×2 och×4. Det nya [galleriet](index.html) visar32 instanser (referensbilder återkommer vid jämförelser), i föreslagen spelskala320×192. Bildvalet och regelpaketet är fortsatt BILD för Timothy.

Timothys senaste precisering styr kompositionen: **stadsmiljö med horisont**, en plats man ser inifrån. Större stad ökar husens höjd, kvartersmassa, trängsel och figurband; den fyller inte igen hela öppningen mot bergen. Samma namngivna referensbyggnader finns kvar. 04 är huvudspår och10 är jämförelse, inga liveändringar.

## EXPERIMENT

| Fråga | Bildprov | Byggarens bedömning |
|---|---|---|
| Hur behålls stadsmiljö och horisont? | Gata och takgård i tre populationer | Den smala öppningen mot himmel/berg består; storlek får i stället påverka sidornas höjd och täthet. Gatans storlek är fortfarande mer indirekt än i en hel stadssiluett. |
| Hur blir svält tydligare? | Samma4500/mur1 i fyra tillstånd | Inga härdar eller vandrande figurer, stängda luckor, hukande figur och tomma skålar. En separat tom-skål-plakett stärker signalen men är ett prövbart UI-lager, inte kanon. |
| Hur känns mur0/1/3? | Full3×3matris per kamera | Ingen mur; låg stenlinje och port; högre blockmur med tinnar, torn och större portkropp. Förgrundshus får delvis skymma försvar men inte hela porten. |
| Vad bär gatans bild? | Huvud/djup/varma-berg | Förskjuten siktpunkt och närmare människor förstärker djup. Varma berg lånar03:s bandade palett utan att förlora gårdens ljusa målpunkt. |
| Vad bär takbilden? | Huvud/djup/varma-berg | Närmare tak ramar in en ljus gård; horisonten ligger bortom tempel och kvarter. |
| Ryms ett skarpt raster på mobilen? |160×96 i heltalszoom2 |320×192ryms i390px-panel med24px på vardera sida+1pxram;340px innehållsbredd,20px kvar. Ingen fraktionell omskalning. |

Kontaktark: [status](status-kontakt.png), [skala](scale-kontakt.png), [varianter](variants-kontakt.png), med motsvarande gråskalefiler. Matrisens kolumner är mur0/1/3, rader1000/4500/10000. De sorteras uttryckligen efter data även när en bild också används i statusgruppen.

## GRINDAR

| Grind | Bevis |
|---|---|
| Kod/artifakter |146 genererade PNG byte-identiska mellan två körningar;56 skalade scenpar exakt NEAREST;28 statuskompositer exakt struktur+RGBA-lager. Statuslagren använder endast alfa0/255. |
| Data | Alla18population/mur-kombinationer ger olika raster vid ändrad parameter; samma strukturraster i varje tillstånd. Det bevisar **databeroende**, inte mänsklig snabbavläsning. |
| Visuell | Byggargranskat i320×192, logisk1:1, gråskala och×4. Firefox146.0.1 vid1440/390,DPR1:32spelskaleinstanser i exakt320×192CSS; inget sidöverflöde, ingen pageerror. Statisk panel med faktisk390px/24pxpadding/1pxram verifierad separat. |
| Semantisk | Population styr band, inte ett hus per invånare. Murnivå styr flera drag. Normal/svält/belägring/bygge är separata lager, med ställning på just templets ankare. Ingen uppfunnen stadsbyggnad eller API-sanning. |
| Användare | Faktiskt Firefoxflöde: öppna galleri→gråskala→390px→öppna logisk/×4→rulla datamatris. Offlineprov, ingen spelserver/registrering/keryxacceptans. |
| Drift | Ingen merge/push/deploy och inga produktionstillgångar ändrade. |

Fyra fysiska mutationer, alla namngivet RÖDA: bilinjär omskalning, fast populationstier, fast murnivå, borttagen tempelställning. Byte-exakt källåterställning och nya genererade kontroller GRÖNA efter var och en. [Mutationsresultat](mutation-results.json), [Firefox-/artifaktbevis](verification.json). Hela spelets Go-/JS-sviter är inte relevanta för denna fristående bildstudie.

### Reproduktion

```sh
python3 docs/reviews/stadsvy-alternativ/21b/generate.py
python3 docs/reviews/stadsvy-alternativ/21b/mutation_check.py
python3 docs/reviews/stadsvy-alternativ/21b/verify.py
```

Kräver Pillow12.3,DejaVuSans.ttf och PlaywrightFirefox (samma miljö som21). Ingen tillgång till server eller levande världen används. Basgit cf4c1ef2; ingahealthz/migrationspåståenden hör till bildmätningen.

## METODISK LÄRDOM

- En gatukamera behöver låta storlek påverka förgrundens höjd och rum, inte bara fler hus bakom samma fasader.
- Räkna inte en pixeldiff som mänsklig läsbarhet; datamatrisen är granskningsmaterial för BILD.
- Pixelbildens spelstorlek ska väljas utifrån faktisk inre panelbredd och heltalszoom före detaljarbete.

## KÄNDA AVGRÄNSNINGAR

Detta är en parameterstyrd **prototyp för två kompositioner**, inte en allmän stadsrenderer. Endast population och mur varieras; alla andra byggnadsdata är referensstadens. Byggnader som sidofunktioner kan delvis skymmas av gatans förgrund — ingen full katalogläsbarhet påstås. Grupperna1000/4500/10000 är visuella band, ingen uppmätt befolkningskalibrering. Människor kan skymmas av den högre förgrundsmuren i takvyn; rastertestet lovar inte att varje figur syns.

Den tomma skålen i förgrunden är en matplats/dekor, inte påstående om en ny marknadsbyggnad. Den extra svältplaketten prövar en explicit signal vid sidan om den fysiska scenen; Timothy kan föredra endast miljötecknen. Inga siffror eller svältregler härleds från en bild. Läger i sidoglugg kan visuellt hamna nära hus även när de representerar land utanför försvarslinjen; geometrin är en symbolisk stadsscen, ingen taktisk karta.

Panelpassformen är ett måttsatt offlineprov. Den bevisar plats vid390px, men integration i den levande drawerns faktiska CSS/scroll/animation återstår i en egen slice efter bildval. Inga gamla förslag, inklusive04/10, har ersatts eller raderats.
