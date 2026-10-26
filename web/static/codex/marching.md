## Giving a march order

**Right-click the destination hex.** The order menu shows what the destination is, which of your units can go and **March →**. Only one unit is selected initially. Select the checkbox beside each named unit you want to order; units sharing a type or location are separate choices. **More** holds stance ([[stances]]), expedition duration and column name.

You can also left-click a hex and use **March here →** — or **Send galleys →** at sea, or **Colonize →** on empty land ([[colonies]]). Hovering those buttons previews the result on the map.

Choose the named units to send and the menu estimates each selected unit’s arrival **before you send the order**. **War → Army → March** selects that unit; click a destination on the map to open the same menu. Ships with different crews or cargo may arrive at different times. The estimate uses the current route and speed; conditions can change before arrival.

If a messenger must first deliver the order, or the destination is unexplored, the forecast says why arrival is not yet known. Redirects also wait for a messenger; their current course continues until delivery. The **Sold** line shows the selected units’ silver drain at home and in the field, and how long their paying city’s treasury lasts after departure; with several paying cities, it shows the shortest treasury duration; spearmen show **0 → 0**. No forecast sends an order.

In Keryx, use `march --unit <id> --target q,r --preview` (or `unit march` with the same flags). Add `--json` for the server forecast. Omit `--preview` when you are ready to send. `--unit` also accepts an exact unit name, for example `--unit "3rd Spearmen of Tylissos"`; names are case-insensitive and an ambiguous name is refused.

**Choose a city quickly:** the order menu's **Destination city** list offers your own cities first, followed by cities you have actually seen, remembered or contacted. Selecting one fills the destination; you can still choose a hex on the map. A city known only through rumours is not a destination yet. The confirmation names the city.

In Keryx, `march --unit <id> --to "Pylos"` chooses that city's exact hex. The same shortcut works for `unit march`, `unit post` and `unit redirect`, including march previews. Use `--to`, `--target q,r` or `--q/--r`, one form at a time. If two known cities share a name, Keryx lists the candidates and asks for coordinates. An unknown city name is refused. Selecting a city does not change which missions are valid there: a patrol still needs coastal water, so patrol has no city shortcut.

A notification tells you when a unit arrives.

## A marching unit is still reachable

Right-click a new destination and a unit already marching shows up in the menu too, marked **on the march → redirect by messenger**. Pick it like any other unit — the order is a **Redirect**, not a fresh march: the unit keeps going on its old course until the Runner reaches it, then turns onto the new one. You can also redirect from the unit’s **War → Army** row, including by typed coordinates.

## Orders to units already in the field travel by Runner

A unit standing in its city hears you at once. A unit out in the field does not — **Recall** and **Redirect** are carried by a [[runners|Runner]], and the game estimates his next meeting with the unit. The horn sounds when the soldiers *receive* the order, not when you give it.

Recall turns an expedition home to its city, where it rejoins the garrison and files its report ([[sight]]). An ordinary march returns to its departure hex. Redirect sends the unit to the destination you choose.

**War → Army → Recall all**, or `keryx recall --all`, sends a separate recall order to each of your marching units.

Recall and redirect need a passable route from the unit’s current position. If its position or the new route cannot be resolved, the order is refused with a reason. If the Runner cannot catch up during the march, it goes to the destination. For a newly sent land order, if the unit has moved again, the Runner keeps the same order and takes a new physical route to catch it. The order takes effect only when they meet. While the Runner follows a moving unit, **War → Army** and `keryx unit list` show the next meeting estimate, which can change; it is not a guaranteed delivery time. If the Runner must wait before continuing, they show that instead of a delivery time. A destroyed unit or a unit you no longer own cannot receive the order. A Runner-delivered refusal appears in Dispatches; the unit keeps its existing course. Check the unit and choose a reachable destination.

## Where you can march

- **A plain march only reaches ground your people have already seen.** Right-click an unseen hex and the order menu offers **Explore** instead — the unit explores the surrounding area and returns home with a report ([[sight]]).
- Crossing the sea needs ships ([[sea]]).
- A march that cannot go on — its path blocked, or impossible — stalls, and you are told why.

## On the road

A journey starts when the unit receives its order. It stays on its starting hex until it reaches the next one, then advances hex by hex along its saved route. Terrain sets the relative time between steps; the total journey keeps its rounded duration. The map and **Keryx → unit list** show the hex it has reached and its planned arrival time. Sight opens as your people advance, and storms or enemies can stop them on the way.

A marching army eats more than one in garrison and cannot forage ([[upkeep]]). It is seen by anyone whose [[sight]] it passes through, and an enemy on watch may stop it — and a [[battle]] begins by itself. **There is no declaration of war.**

## Exploring an area

Choose **Explore** in the map menu; the default expedition duration is ready to use. Open **More** to change the duration in **game days**. The chosen hex is the centre of an area, even in fog. The order summary shows its extent and duration before sending; **War → Army → March** offers the same controls.

It travels first to the exact hex you chose, even if that hex is already seen, then chooses nearby unexplored ground on its own. It turns home by half the duration. It can turn earlier if the area is already explored or nothing further is reachable. **War → Army** and `keryx unit list` show the area, duration, latest turn and home game day, and why it has turned. A Runner-delivered order begins when received, so those game days become known when it starts. On return, Dispatches carries the report ([[sight]]).

The chosen hex is the first destination. If it cannot be reached from this unit's position, the order is refused without substituting another destination. Choose a reachable hex or use a unit departing from elsewhere. After reaching the chosen hex, a ship prefers unseen coastal water over open sea.

In Keryx: `unit march --unit <id> --target q,r --intent explore --ticks N`. Omit `--ticks` for the server's default duration; the current allowed range comes from the game. Add `--preview` to check the order without dispatching. Exploration routes cross unknown ground, so their arrival forecast stays unavailable.

## Terrain and journey time

For foot soldiers, each plain, river valley or delta takes **1 game day**; olive groves
and maquis **1.5 game days**; hills and cedar forest **2 game days**; semi-desert **3 game days**.
A ford takes **2.5 game days** on foot. Mountains cannot be crossed.
War chariots are fast on the flat and slow everywhere else: **0.67 game days** on a
plain, river valley or delta, **2 game days** in olive groves and maquis, and
**3 game days** in hills, cedar forest and semi-desert. A ford takes them 2.5 game days
too. The Nomadic Host
needs twice the foot soldiers' time. A Runner takes half the foot time on land.

A fully crewed Galley sails **3 coastal hexes per game day** or **5 deep-sea hexes per
game day**, **2 river hexes per game day**, and takes **1 game day** through a ford. A War Galley
needs 60% of the Galley's time; an Emporos needs 140%. A short crew or a carried
land unit slows a marching ship. Runners aboard follow their carrier's journey.
Cargo journeys use the separate [[routes|caravan rule]]: 1.5 times the base route
time, including at sea; cargo does not change their speed.

These are base costs before rounding the whole journey to at least one game day.
The route and its costs are fixed at departure. A journey already underway keeps
its recorded costs and arrival game day when the movement rules change. A finished
[[roads|road]] shortens the hex it leads into for everyone who walks it.

An expedition preview shows its planned length. For a unit in garrison it can also show the planned homecoming **if it departed immediately**. The order may still need physical delivery. The preview names the selected first destination; its route and arrival time remain undisclosed.

A march preview applies the same expedition budget checks as the order. Before sending an expedition, both the web menu and Keryx check its duration without dispatching anything. A refused or unavailable validation sends no Runner: choose a longer duration or a nearer area. A valid expedition may still have an unavailable arrival time. Unseen routes and their travel times remain undisclosed. A Host founds with `founding settle`, not a colony march. Without confirmation, nothing is sent.

If you send a new movement order to the same unit while an earlier order Runner still waits unboarded in one of your ports, the earlier order is withdrawn in the same dispatch. That Runner walks home; the new order still travels physically. A Runner already aboard a ship or travelling elsewhere continues on its own course, and an independent stance order is unaffected. Your dispatch receipt says when it withdrew an earlier order.
