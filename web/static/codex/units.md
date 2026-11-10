Forces are the soldiers, ships and people you command.

- Land forces train as a 100-man cohort drafted from one city's people; ships are built one at a time.
- Each force needs its building: Barracks, Stable or Shipyard (Foundry for Bronze Guard and War Galleys).
- Every force eats each game day; Bronze Guard, chariots and War Galleys also draw silver.
- Home service costs half the field pay.
- Open the **War (⚔)** drawer: **Forces**, **Recruit**, **Marches**. <!-- src: server/internal/unit catalogue; reference.json units -->

## Recruitment costs and requirements {#units-recruiting}

**War → Recruit.** Pick the city; an unavailable row names what is missing.

| Force | Needs | Notes |
|---|---|---|
| **[[elite-infantry|Bronze Guard]]** | Barracks + [[foundry]] | Costs [[bronze]]. Far harder per man. |
| **[[war-chariot|Chariot]]** | [[stable]] | Costs bronze. The hardest hitter on the field. |
| **[[emporos|Emporos]]** | Shipyard | 90 cedar; no silver or timber. Trade hull; carries the most goods and up to three land forces, no warship. |
| **[[galley|Galley]]** | [[shipyard|Shipyard]] | 30 [[timber|timber]] (or [[cedar|cedar]]); no silver. Transport and sight at sea ([[sea]]). |
| **[[spearmen|Spearman]]** | [[barracks]] | The levy. 12 [[grain|grain]] for one 100-man cohort; no silver. |
| **[[war-galley|War Galley]]** | Shipyard + Foundry | Costs cedar. The warship. |

Cedar can pay for timber, one for one; timber is used first.

### Cohort training and readiness {#units-recruiting-2}

Land forces train as a whole 100-man cohort (**Train 100**). Ships are built one at a time and can be named; cost and draft cover the ship's crew. Rejected recruitment consumes nothing. A cohort still gathering men or training cannot march; once ready it can march after losing men without refilling to 100.

Every cohort is numbered by its home city: **2nd Spearmen of Knossos**. A lost regiment's number is never reused. Before your first city, your [[horde]]'s two escort cohorts carry your own name and become the city's 1st and 2nd when you settle.

## Drafting and supporting forces {#units-recruits-are-your-own-people}

A cohort is drafted out of your city's population and stays out until disbanded. Soldiers still eat. **Emporoi, Galleys and Spearmen draw 0 silver per game day**. Bronze Guard, chariots and War Galleys draw pay in [[silver]], half at home: a full elite cohort draws 0.75 / 1.5 silver per game day at home / in the field, a full chariot cohort 2 / 4, a full War Galley 0.25 / 0.5 ([[upkeep]]). Rowers eat like spearmen: a port ration at home, double on a mission. An army bigger than your city can feed and pay is a famine with spears.

### Disbanding and supporting city stores {#units-recruits-are-your-own-people-2}

**Disband** takes a whole unit by name (City → Garrison). Its survivors return to that city as people; silver is unchanged. A laid-up hull returns no crew a second time. Keryx: `disband --unit "<unit name>"`; repeat `--unit` for several forces.

### Laying up and fitting out crews {#units-recruits-are-your-own-people-3}

An empty ship in your own active [[harbour|Harbour]] can be **Laid up**: its crew returns to the city's population, and the hull draws no food or silver. **Fit out** takes the full crew from the city again and prepares the ship for **three game days**; it cannot sail or be cancelled meanwhile, and its crew draws home-rate food (and half field pay for a War Galley) from the start. Keryx: `unit lay-up --unit ID`, `unit fit-out --unit ID`.

## Reading the Forces roster {#units-the-forces-tab}

Each unit shows its state and its grain and silver upkeep per game day. A damaged land cohort in its **original home city** can **Reinforce**: at most **4 men per game day**, taken only from that game day's population growth, so the city never shrinks to refill it. Keryx accepts `--to "City name"` on march, post and redirect. See [[marching]], [[stances]] and [[sea]].

**Marches** lists your marches and every foreign column you can see heading for your settlements (**Incoming**), soonest first ([[defence]]).

A foreign force on the map shows only what you observed ([[map-actors]]). Keryx: `keryx hex <q,r>`.

### Cancelling training and cargo capacity {#units-the-forces-tab-2}

While a unit is still training, **Cancel training** (Keryx `unit cancel-training --unit ID`) returns its people and reserved goods; a ship under construction has **Cancel build**. Finished forces use Disband.

A **Galley carries 20** goods and an **Emporos 120**, of any kind; one good takes one cargo place, and a ship carrying a land unit cannot carry freight. Several goods can share one [[transfers|shipment]].

## Forces, Marches and commerce {#units-forces-and-marches}

**Forces** groups your roster as **Land Forces**, **Ships** and, only while you have a Nomadic Host, **People**. A ship bound to a standing route stays under Ships and names its route. Automated trade is under **Economy → Routes & cargo** ([[routes]], [[transfers]]). `keryx forces` shows the same roster.
