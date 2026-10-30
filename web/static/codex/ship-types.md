Every ship needs a **[[harbour|Harbour]]** and a **[[shipyard|Shipyard]]** ([[buildings]]). There are three kinds of ship, recruited like other [[units|Forces]]:

<!-- list: unit galley war_galley merchantman -->
| Name | In short |
|---|---|
| [[galley]] | The Galley is the ordinary ship: built of timber, rowed by twenty, the hull that carries your people and your goods across the Thalassa. |
| [[war-galley]] | The War Galley is the warship: cedar-built, fifty at the oars, fast and dangerous. |
| [[emporos]] | The Emporos is the merchant hull: broad, slow, cedar-built, made to carry. |
<!-- /list -->

The Galley is cheap [[timber|timber]] and no [[silver|silver]]; the War Galley needs [[cedar|cedar]] and a [[foundry]] and never carries cargo; the Emporos is 90 cedar and carries the most. How the sea itself works (storms, sea lanes, the open deep) is in [[sea]].

A [[transfers|transfer]] or [[routes|standing order]] that goes by sea needs a free Galley or Emporos waiting in the sending port — the War Galley is never chosen. With automatic routing, no free hull means land instead if a route exists, or refusal. An explicitly chosen sea route never switches to land.

## Fetching a land force by ship {#fetch-by-ship}

A land [[units|unit]] stranded on another shore — landed there, or marched off on its own — cannot swim home. From that unit's own card (**Fetch by ship**, whenever one of your own ships qualifies), or `keryx unit pickup`, you send a ship to bring it back.

- **The shore is chosen for you.** If the unit already stands on ground a ship can reach by sea, that is the shore. Otherwise the nearest open, unclaimed coastline near it is picked automatically — never a [[foreign-cities|foreign city]].

- **A runner rides along if one is needed.** When the unit does not already stand on the chosen shore, a Runner boards the very same ship, steps ashore with it, and marches inland to order the unit to the shore — exactly like [[messengers|arrange passage]]'s own crossing, just carrying a march order instead of a letter. Only a Galley or Emporos can carry that runner; a War Galley may only fetch a unit that is already standing on the shore.

### Pickup waiting time and capacity {#fetch-by-ship-2}

- **The ship waits, but not forever.** You choose how many game days it holds off the shore. If the unit reaches the shore in time, it boards at once and the ship sails home with it. If the wait runs out first, the ship sails home empty — you are told either way.

- **Home is the end of the errand.** The moment the ship docks, the fetched unit steps off straight into that city's garrison — no separate Unload needed.

Give a pickup mission while the ship is in port. Ships already at sea cannot receive new orders.

## Docking in your own city {#landing-in-own-city}

In **War → Forces**, use **Load** for each cohort you want aboard, then **March** and choose your own coastal city or harbour on the map or in **Destination city**. A Galley or War Galley takes one land unit; an Emporos takes up to three.

The ship sails to the city and docks there. **Everyone aboard joins that city's garrison automatically**, and **the ship stays in its harbour**. You do not need a second Unload order. An empty ship can sail to the same city and stay there. The ship and troops keep their existing supporting city; docking does not transfer who pays their upkeep. **The ship takes provisions for its next voyage from the harbour where it is actually docked**, for ordinary missions as well as freight. A distant supporting city's food cannot supply this departure.

### Landing on open ground {#landing-in-own-city-2}

Keryx: `unit march --unit <ship-id> --to "Petras"`. A laden ship can also use `--intent land` for your own city. For a foreign coastal city, an ordinary march by a laden ship uses the existing amphibious assault; Land does not attack a foreign city. **Land on open, unclaimed ground** still puts everyone ashore and sends the ship home, with optional colony founding. Ships take these orders before departure from port.

## Sailing time {#sailing-time}

With a full crew, a Galley covers **3 coastal hexes** or **5 deep-sea hexes per
game day**. Rivers allow **2 hexes per game day**; a [[river-ford|ford]] takes **1 game day** for a ship. With the current a ship goes in half that time, against it in twice the time; across or where the flow is unknown, as stated.
Deep water is faster, and [[storms|storms]] can still damage or sink a ship.
See [[marching]] for ship, crew and cargo modifiers and journey rounding.

For goods, a **Galley carries 20** and an **Emporos carries 120 goods of any kind**. One good takes one cargo place; several goods can share a single [[transfers|shipment]]. A ship carrying a land unit is unavailable for goods freight.

## Home with food for the way back {#ship-food-return}

A ship at sea **turns home early** when its food is only enough for the voyage home, with one game day in reserve. It uses its actual crew and any carried soldiers to work out the ration. A ship heading to a city it can still reach with its food may continue there for provisions instead.

**You hear the reason when the ship reaches home**, not while it is far away: “Nike came home early — food for the way back only.” The report names the ship and its surviving crew. It does not mean that the crew starved to half strength.

### Hospitality

When a ship reaches a city, **its own or another Wanax’s**, the city gives it food for the voyage home. This applies to messenger passage, trade and freight. The port uses [[grain|grain]], [[fish|fish]] and [[oil|oil]] from its stores, **after keeping enough for its own people’s daily meal**. It gives only what it can spare; an empty port cannot supply food. Hospitality takes no silver and needs no consent. The host gets no notice naming the visiting ship or its owner; the provisions appear as ordinary food use.

At sea, a ship eats the food carried aboard. See [[sea]], [[food]] and [[upkeep]].

## Capture and loss of ships {#ship-loss}

Ships are damaged in [[battle]] and by storms. A damaged ship can be **Repaired** at a Shipyard (**War → Forces → Repair**); you are notified when the work is done. Each point of hull costs **8% of the ship's build price** in its own material and takes **1 game day**. A Shipyard has room for 3, 6 or 10 ships at a time by level, and at most **10 ships** can wait in one city's build queue. <!-- src: server/internal/combat/ship_repair.go; api/handlers/recruit_quote.go --> Ships can also be lost at sea outright — and [[gods|the gods]], when angry, sometimes take one from the harbour ([[kharis]]).

A ship caught carrying cargo (see [[transfers]]) can also be **captured outright** — it changes hands on the spot and sails to the raider's nearest port under its new flag. If your own trade route's home port is gone by the time a ship comes back — burned, [[occupation|occupied]], abandoned — it docks at your next-nearest settlement instead; with none left anywhere, it sits **stranded** wherever it made landfall until you give it a fresh order.

## Giving a ship its whole mission {#ship-missions}

Choose the mission in **War → Forces** before the ship leaves port. Use **Load** first if it will carry soldiers: one land unit on a Galley or War Galley, up to three on an [[emporos|Emporos]].

| Mission | What happens |
|---|---|
| **March to your city** | Docks there and unloads the soldiers; the ship stays. |
| **Land** | Unloads on open ground, then sails home. Select **…and found a colony** to settle there. |
| **Patrol** | Patrols, then returns home. |
| **Explore** | Explores your chosen area and returns; choose a duration in game days. |
| **Passage** | Carries a [[messengers|letter]] or [[runners|order]], without goods cargo. |

When founding, the first loaded unit settles and the others stand beside it. The departure port supplies the colonists' silver; if founding fails or they unload at home, they bring it back. See [[colonies]].

For Explore, the port loads food for the whole expedition. The ship turns home by half the chosen duration. See [[sight]].

### Commands at sea and crew pay {#ship-missions-2}

- **Once it sails, a ship takes no more orders.** March, Recall, Redirect, Stance, Load, Unload — none of them reach a ship that isn't standing in its own port. Give the whole mission before it leaves; its mission determines whether it comes home or stays docked in your destination city.

- **Sight.** A ship reads the open sea out to four hexes but sees only one hex inland.

- **Crew pay.** Only War Galley rowers draw silver: 1 silver per hundred rowers per game day in the field, half at home — **0.5 / 0.25** for a full crew. Galley and Emporos crews draw no silver pay. Repairs and fitting out count as home; marching, posted and freight missions count as field service, including waits between freight legs. A laid-up ship draws no pay. Carried soldiers draw their own field pay ([[upkeep]]).

### Voyage provisions {#ship-missions-3}

The departure port loads **grain, fish and oil**. [[wine|Wine]] and [[livestock]] are not provisions. At sea, the ship eats only what it carries.

- **Rations:** rowers eat at the same per-person rate as [[spearmen]], doubled while marching, posted or freighting. Carried soldiers need their own field rations. See [[upkeep]].
- **Check supplies:** **War → Forces** or `keryx unit list` shows food for N game days and the goods aboard, including during freight waits. “Less than a game day” means the last ration; “out of food” means none remains.
- **Return:** the ship turns home before it lacks food for the return, unless it can reach a city for provisions. See [[ship-food-return]].
- **At port:** visiting ships receive food under [[xenia]]. Unused provisions return as the same goods, not converted to grain. Meals use available food goods proportionally, as in a city.

## Laying up and fitting out a ship {#ship-types-2}

**Lay up** an idle, empty ship in your own active Harbour from War → Forces when you do not need it. The crew joins the port's population, so the ship costs no food or silver and cannot sail, carry goods or take a passenger. **Fit out** drafts its crew again and takes three game days before the ship is ready. The crew draws home-rate food and, for War Galleys, half field pay while fitting out. A laid-up hull follows its port if the city is annexed. See [[units|Forces]] and [[upkeep]].
