## Giving a march order

**Right-click the destination hex.** The order menu shows what the destination is, which of your units can go and **March →**. All available units in the first group are selected; change the amounts to send a different group. **More** holds stance ([[stances]]), expedition duration and column name.

You can also left-click a hex and use **March here →** — or **Send galleys →** at sea, or **Colonize →** on empty land ([[colonies]]). Hovering those buttons previews the result on the map.

Choose how many units to send and the menu estimates each unit’s arrival **before you send the order**. **War → Army → March** selects that unit; click a destination on the map to open the same menu. Ships with different crews or cargo may arrive at different times. The estimate uses the current route and speed; conditions can change before arrival.

If a messenger must first deliver the order, or the destination is unexplored, the forecast says why arrival is not yet known. Redirects also wait for a messenger; their current course continues until delivery. The **Sold** line shows the selected units’ silver drain at home and in the field, and how long their paying city’s treasury lasts after departure; with several paying cities, it shows the shortest treasury duration; spearmen show **0 → 0**. No forecast sends an order.

In Keryx, use `march --unit <id> --target q,r --preview` (or `unit march` with the same flags). Add `--json` for the server forecast. Omit `--preview` when you are ready to send.

A notification tells you when a unit arrives.

## A marching unit is still reachable

Right-click a new destination and a unit already marching shows up in the menu too, marked **on the march → redirect by messenger**. Pick it like any other unit — the order is a **Redirect**, not a fresh march: the unit keeps going on its old course until the Runner reaches it, then turns onto the new one. You can also redirect from the unit’s **War → Army** row, including by typed coordinates.

## Orders to units already in the field travel by Runner

A unit standing in its city hears you at once. A unit out in the field does not — **Recall** and **Redirect** are carried by a [[runners|Runner]], and the game tells you when he will reach the unit. The horn sounds when the soldiers *receive* the order, not when you give it.

Recall turns an expedition home to its city, where it rejoins the garrison and files its report ([[sight]]). An ordinary march returns to its departure hex. Redirect sends the unit to the destination you choose.

**War → Army → Recall all**, or `keryx recall --all`, sends a separate recall order to each of your marching units.

Recall and redirect need a passable route from the unit’s current position. If its position or the new route cannot be resolved, the order is refused with a reason. A Runner-delivered refusal appears in Dispatches; the unit keeps its existing course. Check the unit and choose a reachable destination.

## Where you can march

- **A plain march only reaches ground your people have already seen.** Right-click an unseen hex and the order menu offers **Explore** instead — the unit explores the surrounding area and returns home with a report ([[sight]]).
- Crossing the sea needs ships ([[sea]]).
- A march that cannot go on — its path blocked, or impossible — stalls, and you are told why.

## On the road

A marching army eats more than one in garrison and cannot forage ([[upkeep]]). It is seen by anyone whose [[sight]] it passes through, and an enemy on watch may stop it — and a [[battle]] begins by itself. **There is no declaration of war.**

## Exploring an area

Choose **Explore** in the map menu; the default expedition duration is ready to use. Open **More** to change the duration in **days**. The chosen hex is the centre of an area, even in fog. The order summary shows its extent and duration before sending; **War → Army → March** offers the same controls.

The unit chooses nearby unexplored ground on its own, and turns home by half the duration. It can turn earlier if the area is already explored or nothing further is reachable. **War → Army** and `keryx unit list` show the area, duration, latest turn and home day, and why it has turned. A Runner-delivered order begins when received, so those days become known when it starts. On return, Dispatches carries the report ([[sight]]).

In Keryx: `unit march --unit <id> --target q,r --intent explore --ticks N`. Omit `--ticks` for the server's default duration; the current allowed range comes from the game. Add `--preview` to check the order without dispatching. Exploration routes cross unknown ground, so their arrival forecast stays unavailable.

## Terrain and journey time

For foot soldiers, each plain, river valley or delta takes **1 day**; olive groves
and maquis **1.5 days**; hills and cedar forest **2 days**; semi-desert **3 days**.
A ford takes **2.5 days** on foot. Mountains cannot be crossed.
War chariots are fast on the flat and slow everywhere else: **0.67 days** on a
plain, river valley or delta, **2 days** in olive groves and maquis, and
**3 days** in hills, cedar forest and semi-desert. A ford takes them 2.5 days
too. The Nomadic Host
needs twice the foot soldiers' time. A Runner takes half the foot time on land.

A fully crewed Galley sails **3 coastal hexes per day** or **5 deep-sea hexes per
day**, **2 river hexes per day**, and takes **1 day** through a ford. A War Galley
needs 60% of the Galley's time; an Emporos needs 140%. A short crew or a carried
land unit slows a marching ship. Runners aboard follow their carrier's journey.
Cargo journeys use the separate [[routes|caravan rule]]: 1.5 times the base route
time, including at sea; cargo weight does not change their speed.

These are base costs before rounding the whole journey to at least one day.
The route and its costs are fixed at departure. A journey already underway keeps
its recorded costs and arrival day when the movement rules change.
