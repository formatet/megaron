You see the world at two depths: what you are looking at **now**, and what you **remember**.

## Live sight

Every settlement, every unit standing on the map and every messenger on the road is an eye. A Runner or messenger reveals the ground on the way out *and* on the way home.

| Eye | Sees |
|---|---|
| Settlement | 3 hexes |
| Land unit, or the Nomadic Host | 2 hexes |
| Ship | 1 hex of land — a ship is blind inland |
| Anyone, looking at a mountain | 2 hexes further — mountains are landmarks |
| Anyone standing at open water | 4 hexes of sea |
<!-- src: server/internal/province/hex.go LiveRadius -->

The open horizon belongs to whoever stands at the water. A coastal city or a ship reads the sea out to four hexes; an army two hexes inland does not, however much sea lies over the hill. A river between banks is not the sea and opens no horizon.

## Memory

Everything you have seen and are no longer looking at is drawn **dimmed, frozen as you last saw it**. It can be wrong, and it will not tell you so.

**Foreign units are drawn only on ground you can see live**, never on remembered ground. They are drawn with a blinking outline.

## Exploring

You cannot march an army into land none of your people has ever seen. Push the edge outwards step by step — every hex a unit reaches widens what it sees. At sea, sending a ship to an unknown hex is an **explore** order: the ship sweeps the fog there and sails home by itself, and you are told what it found.

The oracle rite reveals ore deposits you cannot see — see [[rites]]. Seeing a city does not open trade with it — that takes [[contact]].
