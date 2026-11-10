**Right-click a destination**, select your forces and check the forecast before sending.

- A plain march reaches only ground your people have seen; **Explore** covers unknown ground.
- Foot soldiers take 1 game day per plain hex, up to 3 in semi-desert; mountains cannot be crossed.
- Orders to forces in the field travel by Runner and take effect when it reaches them.
- Ships get their whole mission in port ([[ship-types]]).
- An enemy can stop you and a [[battle|battle]] begins by itself: there is no declaration of war. <!-- src: server/internal/province/movement.go TerrainMoveTicks -->

## Selecting forces for a march {#marching-giving-a-march-order}

**Right-click the destination hex.** The order menu shows what the destination is, which of your forces can go and **March →**. Check each named unit to order. **More** holds stance ([[stances]]), expedition duration and column name. **Colonize →** works on empty land ([[colonies]]).

The menu estimates each unit's arrival **before you send**, from the current route and speed.

### March forecasts and city destinations {#marching-giving-a-march-order-2}

If a [[messengers|messenger]] must first deliver the order, or the destination is unexplored, the forecast says why arrival is not yet known. The **Sold** line shows the forces' [[silver|silver]] drain at home and in the field and how long the treasury lasts ([[spearmen|spearmen]] show **0 → 0**).

Keryx: `march --unit <id> --to q,r --preview`; omit `--preview` to send. `--unit` also accepts an exact unit name.

**Destination city** lists your own cities, then cities you have seen, remembered or contacted; a city known only by rumour is not a destination.

### City destinations in Keryx {#marching-giving-a-march-order-3}

`march --unit <id> --to "Pylos"` targets that city's hex (also `unit post`, `unit redirect`). If two known cities share a name, Keryx asks for coordinates. A patrol needs [[coastal-sea|coastal water]], so it has no city shortcut.

## Redirecting a moving force {#marching-a-marching-unit-is-still-reachable}

A unit already marching also appears in the menu, marked **on the march → redirect by messenger**. The order is a **Redirect**: the unit keeps its old course until the Runner reaches it, then turns.

## Recall and Redirect by Runner {#marching-orders-to-forces-already-in-the-field-travel-by-runner}

A unit in its city hears you at once. A unit in the field does not: **Recall** and **Redirect** are carried by a [[runners|Runner]], and the game estimates his next meeting with the unit.

Recall turns an expedition home, where it rejoins the garrison and files its report ([[sight]]); an ordinary march returns to its departure hex. A redirect can also carry an expedition ([[exploration]]). `keryx recall --all` orders every marching force home.

### Recalling all and keeping failed orders {#marching-orders-to-forces-already-in-the-field-travel-by-runner-2}

Recall and redirect need a passable route from the unit's position; otherwise the order is refused with a reason. If the unit has moved again, the Runner keeps the order and takes a new route to catch it; it takes effect only when they meet. The meeting estimate can change and is not a guaranteed time. A destroyed unit, or one you no longer own, cannot receive the order. A refusal arrives in Dispatches and the unit keeps its course.

## Passable destinations {#marching-where-you-can-march}

- **A plain march only reaches ground your people have already seen.** On an unseen hex the menu offers **Explore** instead ([[sight]]).
- Crossing the sea needs ships ([[sea]]); a march that cannot go on stalls and you are told why.
- Which terrain each kind of force can enter: [[terrain]].

## Forces travelling on a route {#marching-on-the-road}

The unit moves hex by hex once it receives its order; sight opens along the way, and [[storms]] and enemies can interrupt it. A marching army eats more than one in garrison and cannot forage ([[upkeep]]). It is seen by anyone whose [[sight]] it passes.

## Ordering an expedition {#marching-exploring-an-area}

Choose **Explore**; **More** changes the duration in **game days**. The chosen hex is the centre of an area, even in fog. The unit travels first to the exact hex, then picks nearby unexplored ground on its own, and turns home by half the duration, or earlier if nothing further is reachable. A Runner-delivered order begins when received. The report comes in Dispatches ([[sight]]).

### Expedition destinations and duration {#marching-exploring-an-area-2}

If the chosen hex cannot be reached from the unit's position, the order is refused; no other destination is substituted. A ship prefers unseen coastal water over open sea.

Keryx: `unit march --unit <id> --to q,r --intent explore --ticks N` (`--preview` checks only). Exploration routes cross unknown ground, so their arrival forecast stays unavailable.

## Land terrain and travel time {#marching-terrain-and-journey-time}

For foot soldiers, each [[river-delta|delta]], plain or [[river-valley|river valley]] takes **1 game day**; [[maquis|maquis]]
and [[olive-grove|olive groves]] **1.5 game days**; [[cedar-forest|cedar forest]] and [[hills|hills]] **2 game days**; [[semi-desert|semi-desert]] **3 game days**.
A [[river-ford|ford]] takes **2.5 game days** on foot. Mountains cannot be crossed.
[[war-chariot|Chariots]] are fast on the flat and slow elsewhere: **0.67 game days** on a
delta, plain or river valley, **2** in maquis and olive groves, **3** in cedar forest, hills and semi-desert, 2.5 at a ford.
The [[horde|Nomadic Host]] needs twice the foot soldiers' time. A Runner takes half the foot time on land.

### Ship speed and expedition forecasts {#marching-terrain-and-journey-time-2}

A fully crewed [[galley]] sails **3 coastal hexes per game day** or **5 deep-sea hexes per
game day**, **2 [[rivers|river]] hexes per game day**, and takes **1 game day** through a ford; with the current half the time, against it twice ([[rivers]]). A [[war-galley]]
needs 60% of the Galley's time; an [[emporos]] 140%. A short crew or a carried land unit slows a ship. Cargo journeys use the separate [[routes|caravan rule]]: 1.5 times the base time, at sea too.

The whole journey is rounded to at least one game day, and its time is fixed at departure. A finished
[[roads|road]] shortens the hex it leads into for everyone who walks it.

### Expedition budget and superseded orders {#marching-terrain-and-journey-time-3}

A preview applies the same budget checks as the order and sends nothing. A Host founds with `founding settle`, not a colony march.

A new movement order to a unit withdraws an earlier order whose Runner still waits unboarded in your port; that Runner walks home. A Runner already aboard or travelling continues, and a stance order is unaffected.
