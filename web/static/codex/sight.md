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

Everything you have seen and are no longer looking at is drawn **dimmed, frozen as you last saw it**. It can be wrong, and it will not tell you so.

**Foreign units are drawn only on ground you can see live**, never on remembered ground. They are drawn with a blinking outline.

## Exploring

You cannot **march** an army into land none of your people has ever seen — but you can **explore** it. Sending a unit to an unseen hex, land or sea, is an **explore** order: it sweeps the fog there and returns home by itself, and you are told what it found. Right-click an unexplored hex and the order menu offers only this — terrain there is unknown, so any unit that can reach it may go. On ground you already know, "Explore" is also offered alongside plain March, if you just want a look without garrisoning.

The oracle rite reveals ore deposits you cannot see — see [[rites]]. Seeing a city does not open trade with it — that takes [[contact]].
