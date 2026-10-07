You see the world at two depths: what you are looking at **now**, and what you **remember**.

## Live sight

Every settlement, every unit standing on the map and every messenger on the road is an eye. A Runner or messenger reveals the ground on the way out *and* on the way home.

| Eye | Sees |
|---|---|
| Settlement | 3 hexes |
| Land unit, or the Nomadic Host | 2 hexes |
| Ship | 1 hex of land — a ship is blind inland |
| Anyone, looking at a mountain | 2 hexes further — mountains are landmarks |
| Anyone standing at open water | 4 hexes of sea — in a straight line across open water only |
<!-- src: server/internal/province/hex.go LiveRadius, SeaSightline -->

The open horizon belongs to whoever stands at the water, and it reaches only across open water in a straight line. A coastal city or a ship reads the sea out to four hexes, but only where nothing but sea lies between it and the hex it looks at: an island, a headland or a strip of shore in the way blocks the view beyond it. An army on the beach does not see a lake behind it over the hills, and an army two hexes inland does not see the sea at all beyond its ordinary two hexes. A river between banks is not the sea and opens no horizon.

## Memory

The map shows **colour for live sight, grayscale for remembered ground, and black for places never seen**. Remembered ground keeps its terrain details, frozen as you last saw it. It can be wrong, and it will not tell you so.

**Foreign units are drawn only on ground you can see live**, never on remembered ground. They are drawn with a blinking outline.

## Exploring

You cannot **march** an army into land none of your people has ever seen — but you can **explore** it. Right-click a hex, seen or unseen, land or sea, and choose **Explore**: the unit sets out as an **expedition** to the land within 5 hexes of that place, for as many days as you give it (4 to 30; 10 if you do not say).

- **It finds its own way.** Each time it stops, it heads for the nearest ground there that none of your people has seen. You watch it go, live, but you cannot steer it — that is the point of sending it.
- **The order reserves time for the journey home.** It turns back no later than halfway through its days, so it is home by the end of them. It turns sooner if nothing in the area is left unseen, or if what is left cannot be reached. You are told when it turns, and why. If its home is lost during the expedition, it seeks another reachable city of yours; its mission line then shows the actual return day. Without a reachable home it stops where it is.
- **It reports when it is home**: how long it was out, how far it went, how much it saw, and what it found — copper, tin, silver and cedar, and the foreign cities on the way.
- An expedition into ground you already know all of is refused, and so is one too short to reach the nearest unseen ground and come back.
- A Runner that recalls or redirects it ends the expedition; it then behaves like any other march.
- Out in the field it eats double, from home, like any unit away from its city ([[upkeep]]). A ship takes on food for every day of the expedition before it sails ([[sea]]).
<!-- src: server/internal/combat/expedition.go, march_start.go (explore) -->

The oracle rite reveals ore deposits you cannot see — see [[rites]]. Seeing a city does not open trade with it — that takes [[contact]].
