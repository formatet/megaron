The sea is **the Thalassa**. It carries your ships, your traders and your ambitions — and it is how you reach the half of [[bronze]] your own land does not have.

## Ships

Every ship needs a **Harbour** and a **Shipyard** ([[buildings]]). There are three kinds ([[units]]):

- **Galley** — 30 timber, no silver to build or maintain. Transport and sight at sea. Can carry goods.
- **War Galley** — the warship. Needs cedar and a Foundry. Never carries cargo.
- **Emporos** — a trade hull built with 90 cedar, no timber or silver. It draws no silver pay. It is no warship (1 die in battle, like a Galley), but carries more than a Galley.

A [[transfers|transfer]] or [[routes|standing order]] that goes by sea needs a free Galley or Emporos waiting in the sending port — the War Galley is never chosen. No free hull there, and the shipment goes by land instead if a road exists, or is refused.

**Lay up** an idle, empty ship in your own active Harbour from War → Army when you do not need it. The crew joins the port's population, so the ship costs no food or silver and cannot sail, carry goods or take a passenger. **Fit out** drafts its crew again and takes three game days before the ship is ready. The crew draws home-rate food and, for War Galleys, half field pay while fitting out. A laid-up hull follows its port if the city is annexed. See [[units]] and [[upkeep]].

## Carry troops to your own city

In **War → Army**, use **Load** for each cohort you want aboard, then **March** and choose your own coastal city or harbour on the map or in **Destination city**. A Galley or War Galley takes one land unit; an Emporos takes up to three.

The ship sails to the city and docks there. **Everyone aboard joins that city's garrison automatically**, and **the ship stays in its harbour**. You do not need a second Unload order. An empty ship can sail to the same city and stay there. The ship and troops keep their existing supporting city; docking does not transfer who pays their upkeep, and that city still supplies the ship's next voyage.

Keryx: `unit march --unit <ship-id> --to "Petras"`. A laden ship can also use `--intent land` for your own city. For a foreign coastal city, an ordinary march by a laden ship uses the existing amphibious assault; Land does not attack a foreign city. **Land on open, unclaimed ground** still puts everyone ashore and sends the ship home, with optional colony founding. Ships take these orders before departure from port.

## At sea

- **A ship is given its whole mission in port, then sails alone.** From **War → Army**, **Load** a land unit onto a ship standing with it (a Galley or War Galley takes one, an [[emporos|Emporos]] up to three), then send it on a **mission**: **Land** puts everyone aboard ashore on open ground and sails home again on its own — select "…and found a colony" to settle right there, no further order needed; the unit loaded first founds it and the others stand beside it. The port pays the colonists' silver the moment the ship sails, as it would for a column on foot ([[colonies]]); if the colony cannot be founded, or you unload them at home, the silver comes back with them. **Patrol** and **Explore** return home by themselves. For **Explore**, choose an area and duration in game days: the port loads provisions for the whole expedition, and the ship turns home by half the duration ([[sight]]). **Passage** is a fourth mission, empty of cargo, given to carry a [[messengers|letter]] or [[runners|order]] — see "Arrange passage" below.
- **Once it sails, a ship takes no more orders.** March, Recall, Redirect, Stance, Load, Unload — none of them reach a ship that isn't standing in its own port. This is the messenger pillar taken to its edge: nothing reaches a ship at sea, not even you. Give the whole mission before it leaves; its mission determines whether it comes home or stays docked in your destination city.
- **Sight.** A ship reads the open sea out to four hexes but sees only one hex inland.
- **Crew pay.** Only War Galley rowers draw silver: 1 silver per hundred rowers per game day in the field, half at home — **0.5 / 0.25** for a full crew. Galley and Emporos crews draw no silver pay. Repairs and fitting out count as home; marching, posted and freight missions count as field service, including waits between freight legs. A laid-up ship draws no pay. Carried soldiers draw their own field pay ([[upkeep]]).
- **Supplies.** Rowers eat the same per-person ration as spearmen, doubled while marching, posted or freighting ([[upkeep]]). The actual surviving crew determines the ration, and any carried soldiers need their own field rations too. A crew eats grain, fish, oil and wine, with the same proportional sharing as the city. The port loads food for the voyage from these four goods; livestock is not ship provision. War → Army shows food for N game days and the goods aboard, including ships carrying goods on a freight route and waiting between its legs. Keryx `unit list` shows the same supply. Unused provisions come home as the goods that were loaded, so fish does not turn into grain. A ship turns home before its food runs too low for the voyage back, unless it can reach a city for provisions. You hear why when it reaches home; see [[ship-food-return]]. Ports feed visiting ships as guests; see [[xenia]]. At sea it eats only what was loaded aboard, never from a city's granary. The food gauge counts whole game days; "less than a game day" means the last ration is being eaten, "out of food" means nothing is left.

## Storms

The open sea is never quite safe, but **you can see its storms** — see [[storms]]. A storm is three sea hexes of dark cloud that drift slowly across the Thalassa; a ship that crosses any of them takes the damage, even between the game day's start and end, whatever the ship is doing — a fleet, a ship carrying an army, an expedition, a galley carrying a trade, a transfer or a gift. A storm can also drift onto a slow ship. Whose ship it is and why it sails never change that. Rivers and land have no storms. <!-- src: server/internal/combat/sea_storm_weather.go SeaHexesPerStorm -->

- **Each storm costs the ship two points of hull** (of five), once, however many game days it spends inside that storm. Three storms sink a ship that began with full hull. You are told where it struck and how much hull is left.
- **At hull 0 the ship founders.** It goes down with everything aboard: its crew, any troops it carries, and any cargo. You are told where, and what was lost with her.
- A battered ship keeps sailing. Bring it home and **Repair** it at a Shipyard before its next long voyage.

## Damage and loss

Ships are damaged in [[battle]] and by storms. A damaged ship can be **Repaired** at a Shipyard (**War → Army → Repair**); you are notified when the work is done. Ships can also be lost at sea outright — and the gods, when angry, sometimes take one from the harbour ([[kharis]]).

A ship caught carrying cargo (see [[transfers]]) can also be **captured outright** — it changes hands on the spot and sails to the raider's nearest port under its new flag. If your own trade route's home port is gone by the time a ship comes back — burned, occupied, abandoned — it docks at your next-nearest settlement instead; with none left anywhere, it sits **stranded** wherever it made landfall until you give it a fresh order.

## Hitching a ride — how a letter or an order crosses the sea

A [[messengers|letter]] or a [[runners|Runner]] carrying your order cannot swim, and it never guesses at a crossing — it needs a real ship, full stop. When its road crosses open water:

- It runs to **your own nearest coastal or harboured city** and waits there — you are told which one, and why. If you have no coastal or harboured city anywhere, the message is refused outright at the moment you send it, rather than dispatched to wait forever.
- The moment **any** ship or trade route — yours or another Wanax's — sails from that port toward the right side of the sea, it **boards** — free, no cargo space taken — and steps off wherever that ship lands, then finishes the trip on foot. Runners are sacred: every captain takes them aboard, even between Wanaxes at war. It only boards a ship that lands it **nearer its goal** than the port it waits in — a ship bound the wrong way, it lets sail. The runner never changes where the ship goes, and the ship's owner sees only that a runner of yours is aboard, never what it carries.
- A storm can [[runner-lost-at-sea|lose the runner]]. In battle a surviving enemy ship [[runner-rescued-at-sea|rescues it]]; a captured ship keeps it aboard. It leaves that actual ship in its next port and continues. A damaged ship that survives takes its runner home to the next port, without sealing or loss.
- If no ship sails that way for a while, it simply keeps waiting — nothing crosses on its own. See "A runner stuck waiting" below for what you can do about it.

None of this changes anything for a message or order whose road stays on dry land — a Runner on land never needs a ship, only [[marching|the roads]] it can already walk (a river still needs a ford or a boat of its own, unrelated to sea passage).

## What the map shows while it crosses

Your own [[messengers|letter]] or [[runners|Runner]] is drawn exactly where it really is, not sailing on its own across open water:

- **Waiting** — it stands still in the port it ran to, a gold pennant marking it as yours. A small red mark on top means a **PassageStalled** dispatch has already fired for this wait.
- **Aboard** — it rides the deck of whichever ship or trade route picked it up, moving exactly when and where that ship does. Hover it to see the carrier's name.
- **Ashore** — once the ship lands it, it steps off and finishes the last stretch on foot, same as any Runner on land.
- **No word** — after a rescue its location is unknown to you: no marker, carrier name or sight. It tells you about the detour only when it physically comes home. Sealed markers remain a recovery measure for older unresolved journeys.

## A runner stuck waiting — your decision, not the game's

Wait long enough with no ship sailing the right way, and you get a **PassageStalled** dispatch: the runner, the port it's standing in, where it's trying to reach, and the ships (if any) of yours already there. Nothing forces your hand — it keeps waiting exactly where it is until you act, and the dispatch only ever fires once per spell of waiting (a later known waiting spell can trigger it again; an unknown rescued runner sends no news from a foreign port). Three choices:

- **Arrange passage** — the same button as above, right there in the dispatch.
- **Call it back** — give up on this errand and bring the runner home undelivered. See the instructions below.
- **Let it wait** — close the dispatch and do nothing. It keeps waiting, exactly as it was.

## Call it back — bring the runner home undelivered

In **Diplomacy → Correspondence**, use **Call it back** on the waiting runner's row, or use the same choice in its **PassageStalled** dispatch. In Keryx, find the runner with `outbox`, then use `call-back --id <messenger-id>`.

This works only for an outbound runner waiting for a ship in **your own port**. A runner already aboard a ship, or waiting in a foreign port on its return journey, cannot be called back this way — arrange passage instead.

The runner turns around and walks home over land, delivering nothing. The confirmation tells you when it will return; it is still travelling until then. A letter comes home undelivered, and an order comes home withdrawn.

## Arrange passage — send a ship for your own runner

Waiting for a ship to happen to sail the right way is not your only choice. From your **outbox** (Diplomacy, or `keryx outbox`), a runner waiting for passage shows an **Arrange passage** button (or `keryx passage`) whenever one of your own ships qualifies — the choice of ship is always yours to make, but only real, eligible ships are ever offered: a Galley or Emporos (never a War Galley), standing free in port with no mission of its own.

- **Sending one out:** the ship must stand in the SAME port the runner is waiting in. It sails straight for the runner's destination, boards it the moment it leaves, and puts it ashore there — no different from boarding any ship that happened to be going that way, except this one you chose yourself.
- **Bringing one home:** a runner that has finished its errand and is waiting in a **foreign** port may be fetched instead — any of your own ports will do. The ship sails there, waits (**War → Army** shows it "waiting at (…) for the runner's return", or `keryx unit` says the same), and carries the runner all the way home the moment it is ready to travel.
- Either way, **the ship still finishes its own mission on its own** afterwards — it always sails home again, exactly like Land, Patrol or Explore. The runner never commands the ship; you send the ship for the runner, never the other way round. While it sails out, the drawer and `keryx unit` show it "carrying a runner to (…)".
- The ship's own risks are unchanged: it can still be damaged, captured or sunk, and a starved crew still turns it for home ([[upkeep]]) — none of that touches the runner, who stands safely ashore the whole time.

## Fetch home — send a ship to fetch a unit

A land [[units|unit]] stranded on another shore — landed there, or marched off on its own — cannot swim home. From that unit's own card (**Fetch by ship**, whenever one of your own ships qualifies), or `keryx unit pickup`, you send a ship to bring it back.

- **The shore is chosen for you.** If the unit already stands on ground a ship can reach by sea, that is the shore. Otherwise the nearest open, unclaimed coastline near it is picked automatically — never a foreign city.
- **A runner rides along if one is needed.** When the unit does not already stand on the chosen shore, a Runner boards the very same ship, steps ashore with it, and marches inland to order the unit to the shore — exactly like [[messengers|arrange passage]]'s own crossing, just carrying a march order instead of a letter. Only a Galley or Emporos can carry that runner; a War Galley may only fetch a unit that is already standing on the shore.
- **The ship waits, but not forever.** You choose how many game days it holds off the shore. If the unit reaches the shore in time, it boards at once and the ship sails home with it. If the wait runs out first, the ship sails home empty — you are told either way.
- **Home is the end of the errand.** The moment the ship docks, the fetched unit steps off straight into that city's garrison — no separate Unload needed.

None of this reaches a ship already at sea (ships take no orders once they sail — see "At sea" above): a pickup is a whole mission, given from port, same as Land, Patrol or Explore.

## Sailing time

With a full crew, a Galley covers **3 coastal hexes** or **5 deep-sea hexes per
game day**. Rivers allow **2 hexes per game day**; a ford takes **1 game day** for a ship.
Deep water is faster, and [[storms|storms]] can still damage or sink a ship.
See [[marching]] for ship, crew and cargo modifiers and journey rounding.

For goods, a **Galley carries 20** and an **Emporos carries 120 goods of any kind**. One good takes one cargo place; several goods can share a single [[transfers|shipment]]. A ship carrying a land unit is unavailable for goods freight.
