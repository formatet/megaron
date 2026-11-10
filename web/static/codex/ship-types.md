Every ship needs a **[[harbour|Harbour]]** and a **[[shipyard|Shipyard]]** ([[buildings]]).

- A Galley ([[timber|timber]]) carries 20 goods or one land unit.
- An Emporos (90 [[cedar|cedar]]) carries 120 goods or up to three land forces.
- A War Galley (cedar, [[foundry]]) fights and never carries cargo.
- A ship takes its whole mission in port; once it sails it takes no more orders.
- It eats what it carries and turns home before the food runs short. <!-- src: server/internal/unit catalogue; reference.json -->

## The three ships

<!-- list: unit merchantman galley war_galley -->
| Name | In short |
|---|---|
| [[emporos]] | The Emporos is the merchant hull: broad, slow, cedar-built, made to carry. |
| [[galley]] | The Galley is the ordinary ship: built of timber, rowed by twenty, the hull that carries your people and your goods across the Thalassa. |
| [[war-galley]] | The War Galley is the warship: cedar-built, fifty at the oars, fast and dangerous. |
<!-- /list -->

A [[transfers|transfer]] or [[routes|standing order]] by sea needs a free Galley or Emporos waiting in the sending port; the War Galley is never chosen. Automatic routing falls back to land if no hull is free; a chosen sea route never does. 

## Fetching a land force by ship {#fetch-by-ship}

A land [[units|unit]] stranded on another shore cannot swim home: **Fetch by ship** on its card, or `keryx unit pickup`, sends a ship for it.

- **The shore is chosen for you:** where the unit already stands if a ship can reach it, otherwise the nearest open, unclaimed coast, never a [[foreign-cities|foreign city]].
- **A Runner rides along if needed** and orders the unit to the shore ([[messengers|arrange passage]]); only a Galley or Emporos can carry him, so a War Galley fetches only a unit already on the shore.

### Pickup waiting time and capacity {#fetch-by-ship-2}

- **The ship waits** for the number of game days you choose. If the unit arrives in time it boards and the ship sails home with it; otherwise it sails home empty.
- **Home ends the errand:** the unit steps into the city's garrison.

## Docking in your own city {#landing-in-own-city}

**Load** each cohort, then **March** to your own coastal city or harbour. A Galley or War Galley takes one land unit; an Emporos up to three. **Everyone aboard joins that city's garrison** and **the ship stays in the harbour**. **Provisions for the next voyage come from the harbour where it is docked**.

### Landing on open ground {#landing-in-own-city-2}

A laden ship marching on a foreign coastal city makes an amphibious assault; Land does not attack. **Land on open ground** puts everyone ashore and sends the ship home, optionally founding a colony.

## Sailing time {#sailing-time}

With a full crew a Galley covers **3 coastal hexes** or **5 deep-sea hexes per game day**; [[rivers|rivers]] allow **2 hexes**, a [[river-ford|ford]] takes **1 game day**. With the current, half the time; against it, twice. [[storms|Storms]] can damage or sink a ship. See [[marching]] for modifiers.

## Home with food for the way back {#ship-food-return}

A ship at sea **turns home early** when its food is only enough for the voyage home plus one game day. It may instead continue to a city it can still reach for provisions. You hear the reason when the ship reaches home.

### Hospitality

In any port, **its own or another Wanax's**, a ship is given food for the voyage home: [[fish|fish]], [[grain|grain]] and [[oil|oil]] the city can spare after its own daily meal. It takes no [[silver|silver]] and needs no consent ([[xenia]]).

At sea, a ship eats the food carried aboard. See [[sea]], [[food]] and [[upkeep]].

## Capture and loss of ships {#ship-loss}

Ships are damaged in [[battle]] and by storms. A damaged ship can be **Repaired** at a Shipyard (**War → Forces → Repair**). Each point of hull costs **8% of the ship's build price** in its own material and takes **1 game day**. A Shipyard repairs 1, 2 or 3 ships at a time by level, and at most **10 ships** can wait in one city's build queue. <!-- src: server/internal/combat/ship_repair.go; api/handlers/recruit_quote.go --> Angry [[gods|gods]] sometimes take one from the harbour ([[kharis]]).

A ship carrying cargo ([[transfers]]) can be **captured outright**: it changes hands and sails to the raider's nearest port. If a route's home port is gone ([[occupation|occupied]], burned, abandoned), the ship docks at your next-nearest settlement; with none left it sits **stranded** until you give it an order.

## Giving a ship its whole mission {#ship-missions}

| Mission | What happens |
|---|---|
| **Explore** | Explores your chosen area and returns; choose a duration in game days. |
| **Land / March to your city** | See Docking and Landing above. |
| **Passage** | Carries a [[messengers|letter]] or [[runners|order]], without goods cargo. |
| **Patrol** | Patrols, then returns home. |

When founding, the first loaded unit settles and the others stand beside it ([[colonies]]). For Explore the ship turns home by half the duration ([[sight]]).

### Commands at sea and crew pay {#ship-missions-2}

- **Once it sails, a ship takes no more orders** (Load, March, Recall, Redirect, Stance, Unload). Give the whole mission first.

- **Sight.** A ship reads the open sea out to four hexes but sees only one hex inland.

- **Crew pay.** Only War Galley rowers draw silver: 1 silver per hundred rowers per game day in the field, half at home — **0.5 / 0.25** for a full crew. Galley and Emporos crews draw no silver pay. Repairs and fitting out count as home; marching, posted and freight missions (waits included) as field service. A laid-up ship draws no pay ([[upkeep]]).

### Voyage provisions {#ship-missions-3}

The departure port loads **fish, grain and oil** ([[wine|Wine]] and [[livestock]] are not provisions); at sea the ship eats only what it carries.

- **Rations:** rowers eat at the [[spearmen|spearmen]] rate, doubled while marching, posted or freighting; carried soldiers need their own ([[upkeep]]). **War → Forces** shows food for N game days.
- **Return and port food:** see above and [[xenia]]. Unused provisions return as the same goods.

## Laying up and fitting out a ship {#ship-types-2}

**Lay up** an idle, empty ship in your own active Harbour: the crew rejoins the population, and the ship costs no food or silver and cannot sail. **Fit out** drafts the crew again and takes three game days ([[units]]).
