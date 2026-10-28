You cannot **march** an army into land none of your people has ever seen — but you can **explore** it. Right-click a hex, seen or unseen, land or sea, and choose **Explore**: the unit sets out as an **expedition** to the land within 5 hexes of that place, for as many game days as you give it (4 to 365; 10 if you do not say).

- **First to your chosen Province.** The expedition travels to the exact hex you selected, then explores around it. If it cannot reach that hex, the order is refused; it does not substitute another destination. **After arrival, it finds its own way.** Each time it stops, it heads for the nearest ground there that none of your people has seen. **A ship keeps to the coast:** it makes for unseen [[coastal-sea|coastal water]] before open sea, so the voyage maps the shoreline (it still sails at its usual speed). <!-- src: server/internal/combat/expedition.go nextExpeditionLeg --> You watch it go, live, but you cannot steer it — that is the point of sending it.

## An expedition’s return budget {#exploration-2}

- **The order reserves time for the journey home.** It turns back no later than halfway through its game days, so it is home by the end of them. It turns sooner if nothing in the area is left unseen, or if what is left cannot be reached. You are told when it turns, and why. If its home is lost during the expedition, it seeks another reachable city of yours; its mission line then shows the actual return game day. Without a reachable home it stops where it is.

- **It reports when it is home**: which city it came home to, how long it was out, how far it went, how much it saw, and what it found — [[copper|copper]], [[tin|tin]], [[silver|silver]] and [[cedar|cedar]], and the foreign cities on the way.

- An expedition into ground you already know all of is refused, and so is one too short to reach the chosen hex and come back.

## Recalling an expedition {#exploration-3}

- A [[runners|Runner]] that recalls it turns it home at once; it still files its report on arrival, covering what it saw so far. A Runner that redirects it ends the expedition; it then behaves like any other march.

- Out in the field it eats double, from home, like any unit away from its city ([[upkeep]]). A ship takes on food for every game day of the expedition before it sails ([[sea]]).
<!-- src: server/internal/combat/expedition.go, march_start.go (explore) -->

The oracle rite reveals ore deposits you cannot see — see [[rites]]. Seeing a city does not open trade with it — that takes [[contact]].
