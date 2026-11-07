Sight determines which ground and moving people you can currently see.

You see the world at two depths: what you are looking at **now**, and what you **remember**.

## Live sight {#live-sight}

Every [[messengers|messenger]] on the road, every settlement and every unit standing on the map is an eye. A [[runners|Runner]] or messenger reveals the ground on the way out *and* on the way home.

| Eye | Sees |
|---|---|
| Anyone, looking at a mountain | 2 hexes further — mountains are landmarks |
| Anyone standing at open water | 4 hexes of sea — in a straight line across open water only |
| Land unit, or the [[horde|Nomadic Host]] | 2 hexes |
| Settlement | 3 hexes |
| [[the-bird-oracle|Bird Oracle]] | 2 hexes around one named place, for 1 game day, whatever the ground |
| Ship | 1 hex of land — a ship is blind inland, but sees the shoreline across the water it sees |
<!-- src: server/internal/province/hex.go LiveRadius, SeaSightline -->

### Seeing shorelines from ships {#live-sight-2}

The open horizon belongs to whoever stands at the water, and it reaches only across open water in a straight line. A coastal city or a ship reads the sea out to four hexes, but only where nothing but sea lies between it and the hex it looks at: an island, a headland or a strip of shore in the way blocks the view beyond it. An army on the beach does not see a [[lake|lake]] behind it over the [[hills|hills]], and an army two hexes inland does not see the sea at all beyond its ordinary two hexes. A [[rivers|river]] between banks is not the sea and opens no horizon. **A ship also sees the shore:** every coast hex that borders water on its horizon shows, so sailing along a coast maps the coastline — but not the land behind it. <!-- src: server/internal/province/hex.go SetSeaHorizons -->

## Map memory {#map-memory}

The map shows **colour for live sight, grayscale for remembered ground, and black for places never seen**. Remembered ground keeps its [[terrain|terrain]] details, frozen as you last saw it. It can be wrong, and it will not tell you so.

**A city on remembered ground is shown as you last saw it**: its name, its ruler, its walls and its size, with the time you saw it. If it has changed hands or burned since then, you will not know until someone sees it again. A city founded on ground you saw while it was empty does not appear in your memory, because you remember the empty ground. A [[messengers|messenger]] that reaches a city also lets you see it at that moment. <!-- src: server/internal/province/seen_settlements.go RecordSeenSettlements -->

**Foreign forces are drawn only on ground you can see live**, never on remembered ground. They are drawn with a blinking outline.

## Moving actors on the map {#map-actors}

Your marching armies, [[transfers|caravans]], Runners and ships always show their **remaining route ahead**, beginning where the actor is now. The line shrinks as they travel; nothing is drawn behind them. Solid lines mark armies, mixed long/short dashes mark caravans, dots mark Runners, and long dashes mark ships. The line shows only ground you can see live.

A foreign actor shows only a **compass arrow at its current live position**. It tells you which way the actor goes now, never its destination or full route. Your lines are azure; foreign arrows use the foreign unit's muted accent. Line patterns and arrow shapes carry meaning alongside colour. An agreed trade caravan coming to you shows its remaining route too, in the foreign accent. Its known cargo and arrival remain in the trade details. Gifts are not agreed deliveries and show only an arrow. A Runner coming to you from another Wanax also shows only an arrow. Waiting or sealed Runners have no forward route, and a Runner aboard follows the real carrier. Runners remain sacred and cannot be attacked.
