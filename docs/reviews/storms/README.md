# BILD · Stormar (`claude/stormar`)

En storm är tre sammanhängande sjöhexar mörkt moln. En av hexarna kliver en hex per tick, och
stormen förblir en enhet. Den syns live i sikt, och är annars ett blekt spöke där du sist såg den.

- `firefox-1to1.png` — Firefox, 1280×800, acceptansriggen @ `c59ea87`, mig 168, Hexkolls Kydonia.
  Live storm väster om staden (35,32)(35,33)(34,33) och en till sydost (38,34)(39,33)(39,34).
- `firefox-crop-x3.png` — västra stormen, 3× (nearest).

Fixtur: västra stormens spår är flyttat med SQL till hexarna nära Kydonia (en av 52 stormar skapade
av scanen); en annan storm har en minnespost för Hexkoll (`player_storm_sightings`, sedd för 12 ticks
sedan, ritas som blekt spöke när den ligger utom sikt). Riggen kör vidare: stormarna kryper ett steg
per tick (120 s).

Att döma (BILD): moln, kontur bara runt stormens yttre kant, att de syns över havet men under skepp,
läsbarheten vid 1:1. Spöket (utom sikt) och hover-texten ("Storm — drifting SE" / "last seen on day N")
syns i riggen genom att svepa kartan bort från staden.
