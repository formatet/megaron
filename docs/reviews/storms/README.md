# BILD · Stormar (`claude/stormar`)

En storm är tre sammanhängande sjöhexar mörkt moln. En av hexarna kliver en hex per tick, och
stormen förblir en enhet. Den syns live i sikt, och är annars ett blekt spöke där du sist såg den.

- `firefox-1to1.png` — Firefox, 1280×800, acceptansriggen @ `c59ea87`, mig 168, Hexkolls Kydonia.
  Två live stormar, en väster och en sydost om staden. De har kryppt några steg sedan fixturen sattes (en hex per tick) — det syns på att formerna skiljer sig från startpositionerna.
- `firefox-crop-x3.png` — västra stormen, 3× (nearest).

Fixtur: västra stormens spår är flyttat med SQL till hexarna nära Kydonia (en av 52 stormar skapade
av scanen); en annan storm har en minnespost för Hexkoll (`player_storm_sightings`, sedd för 12 ticks
sedan, ritas som blekt spöke när den ligger utom sikt). Riggen kör vidare: stormarna kryper ett steg
per tick (120 s).

Att döma (BILD): violett moln med regnstråk och blixt (inte bergsgrått), kontur bara runt stormens yttre kant, att de syns över havet men under skepp,
läsbarheten vid 1:1. Spöket (utom sikt) och hover-texten ("Storm — drifting SE" / "last seen on day N")
syns i riggen genom att svepa kartan bort från staden.
