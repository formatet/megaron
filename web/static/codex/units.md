Forces are the soldiers, ships and people you command.

The **War (⚔)** drawer has three tabs: **Forces**, **Recruit** and **Marches**.

## Recruitment costs and requirements {#units-recruiting}

**War → Recruit.** Choose the city at the top if you hold several. Each row shows the total cost of one order and how many men or crew it drafts from the city. An unavailable row keeps that price visible and names what is missing, including the required amount and what the city has.

| Force | Needs | Notes |
|---|---|---|
| **[[spearmen|Spearman]]** | [[barracks]] | The levy. 12 [[grain|grain]] for one 100-man cohort; no silver. |
| **[[elite-infantry|Bronze Guard]]** | Barracks + [[foundry]] | Costs [[bronze]]. Far harder per man. |
| **[[war-chariot|Chariot]]** | [[stable]] | Costs bronze. The hardest hitter on the field. |
| **[[galley|Galley]]** | [[shipyard|Shipyard]] | 30 [[timber|timber]] (or [[cedar|cedar]]); no silver. Transport and sight at sea ([[sea]]). |
| **[[war-galley|War Galley]]** | Shipyard + Foundry | Costs cedar. The warship. |
| **[[emporos|Emporos]]** | Shipyard | 90 cedar; no timber or silver. Trade hull; carries the most goods and up to three land forces, no warship. |

Timber costs can be paid with cedar, one for one. Timber is used first; cedar fills the shortfall. Cedar specifically required by an order must still be available before any spare cedar can cover timber.

### Cohort training and readiness {#units-recruiting-2}

Land forces train as a whole hundred-man cohort (**Train 100**). Ships are built one at a time and can be named. Their cost and population draft cover the ship's crew, rather than a hundred-man cohort. Rowers eat like spearmen: the actual surviving crew draws a port ration at home and double on a mission ([[upkeep]]). Rejected recruitment consumes no people or goods. The completion dispatch names the ready unit type and its city — for example, “Emporos ready in Tylissos”. **Show the ship** or **Review the unit** opens its highlighted Forces card. If the unit no longer exists, the dispatch does not offer an empty unit view. A cohort still gathering men or training cannot march. Once ready, it can still march after losing men; it does not have to refill to 100 first.

Every cohort is numbered by its home city: **2nd Spearmen of Knossos**. A lost regiment's number is never given out again. Before you found your first city, your [[horde]]'s two escort cohorts carry your own name instead — **1st** and **2nd Spearmen of** *you* — and become the new city's 1st and 2nd when you settle.

## Drafting and supporting forces {#units-recruits-are-your-own-people}

A cohort is drafted out of your city's population and stays out of it until disbanded. Soldiers still eat. **Spearmen, Galleys and Emporoi draw 0 silver per game day**. Bronze Guard, chariots and War Galleys draw pay in [[silver]]. Home service costs half field pay: a full elite cohort draws 0.75 / 1.5 silver per game day at home / in the field, and a full chariot cohort 2 / 4. A full War Galley draws 0.25 / 0.5 ([[upkeep]]). An army bigger than your city can feed and pay is not a strong army; it is a famine with spears.

In each combat round, **100 active spearmen roll 50 dice**. Half the active men roll, rounded up: one man rolls one die, three roll two. Bronze Guard rolls three dice per active man and chariots five per chariot. A **War Galley rolls 5 dice**, against a **Galley's 1** before hull damage and other [[battle|battle]] adjustments.

### Disbanding and supporting city stores {#units-recruits-are-your-own-people-2}

**Disband** selects a whole unit by name in City → Garrison. Its remaining people return to that city: a full Spearmen cohort returns 100 people; a damaged cohort returns only its survivors. There is no soldier-quantity input. Ships return their actual crew; a laid-up hull returns no crew a second time. Keryx: `disband --unit "<unit name>"` or `disband --unit <unit-id> --province <city>`; repeat `--unit` to select several whole forces in that city. The receipt names each unit before its returned people and city: “2nd Spearmen of Phaistos disbanded — 41 people return to Phaistos.” Selecting several forces gives one sentence per unit. Silver is unchanged.

### Laying up and fitting out crews {#units-recruits-are-your-own-people-3}

An empty ship in your own active [[harbour|Harbour]] can be **Laid up** from its Forces card. Its crew returns to the port's population and can work there; the hull draws no food or silver and remains in port. The card says **Laid up** and offers **Fit out**. Fitting out takes the ship's full crew from that city's current population and prepares it for **three game days**. It cannot sail or be cancelled during that time, and its crew draws home-rate food and, for a War Galley, half field pay from the start. If the city is annexed, its laid-up and fitting-out ships go with the city. Disbanding a laid-up ship removes the hull without returning the same crew twice; choose the particular hull to disband by its name. Keryx: `unit lay-up --unit ID` and `unit fit-out --unit ID`.

## Reading the Forces roster {#units-the-forces-tab}

Every unit you own, with its state and current grain and silver upkeep per game day in plain words. **March** is shown while it can move; a damaged land cohort garrisoned in its **original home city** can also **Reinforce** — another city you own does not qualify. Reinforcing adds at most **4 men per game day**, taken only from that game day's population growth, so the city never shrinks to refill a cohort. A land unit on the march shows **Recall**. The card also shows stance and **Set**, battle retreat, **Redirect**, **Fetch by ship**, **Load** / **Unload** or **Repair**, whenever that unit can use them. Choose **Destination city** in a march or redirect form to target a known city quickly; map clicks remain available. Keryx accepts `--to "City name"` on march, post and redirect. See [[marching]], [[stances]] and [[sea]].

**Marches** lists your forces on the march (**Your Marches**) and every foreign unit you can see marching on one of your settlements (**Incoming**), soonest arrival first. You only see an incoming column once one of your eyes has it in sight — the *foreign march sighted* notification fires at the same moment ([[defence]]).

**Forces on a hex:** select their current hex on the map to inspect the same force cards, your own first. A marching force appears where it is now. Ships show hull, passengers and available provision information; orders and their destination remain visible. A pending [[messengers|messenger]] is still carrying an order, which takes effect when delivered. **View in Forces** opens the selected force. **Redirect** selects that force and asks for a new destination; **Recall** sends its existing return order. Ships on missions at sea take neither command. On a narrow screen, expand a force's row for details. Your own cities continue to open the City drawer.

`keryx hex <q,r>` lists the forces at that coordinate and the corresponding commands. A foreign force shows only observed information: owner, kind, strength, status, heading and any reported passengers. Its hull, provisions, orders and actual destination are unknown; a movement that *seems* bound for one of your cities remains an estimate. See [[map-actors]].

### Cancelling training and cargo capacity {#units-the-forces-tab-2}

Recruitment confirms that your order was received. While a unit is still training, use **Cancel training** in War → Forces to return its people and reserved resources. A ship under construction has **Cancel build**. In Keryx use `unit cancel-training --unit ID`. Finished forces use Disband instead.

For goods, a **Galley carries 20** and an **Emporos carries 120 goods of any kind**. One good takes one cargo place; several goods can share a single [[transfers|shipment]]. A ship carrying a land unit is unavailable for goods freight.

## Forces, Marches and commerce {#units-forces-and-marches}

**Forces** groups your roster as **Land Forces**, **Ships** and, only while you
have a Nomadic Host, **People**. Being in harbour, marching, aboard another
ship or laid up is shown on each card. A ship bound to a standing route stays
under Ships and names its route, including between legs.

**Marches** lists your ordered marches, sailings, landings, expeditions and
returns, together with known incoming threats. Automated trade is under
**Economy → Routes & cargo**, where each standing route shows its travelling
ship or caravan. Other shipments, including agreements, transfers and gifts,
have their own cargo list there. See [[routes]] and [[transfers]].

`keryx forces` shows the same roster. Older `units`, `army` and `unit list`
commands remain accepted.
