A **storm** is three connected hexes of dark cloud on the Thalassa. There is about one for every 100 hexes of sea, and they never stop wandering. No storm forms on a [[lake]] or drifts onto one. <!-- src: server/internal/combat/sea_storm_weather.go SeaHexesPerStorm -->


Select a water hex on the world map or in the city's workforce grid to read any known storm there. An open panel follows incoming weather updates; workers stay placed while a storm stops fishing. Keryx `storms` lists known storm coordinates, and `status` reports affected fishing grounds.

## How storms move {#storms-how-they-move}

Each game day a storm takes **two steps**. In each step, one of its three hexes moves to a neighbouring sea hex, and the storm stays in one piece. So a storm crawls: slowly, and a little more each game day in the direction it has been drifting, now and then turning. It never crosses land.

## Storm effects {#storms-what-they-do}

A ship that **sails into a storm** takes two points of hull damage (of five) — once per storm, however many game days it spends inside it; three storms sink a ship from full hull. At hull 0 she founders with everything aboard ([[sea]]). A ship beside a storm is safe. Ships follow the route they were given and cannot steer round a storm once they sail ([[sea]]), so look before you send them.

Every sea hex a ship crosses is checked, even when it crosses several in one game day. Two different storms crossed that game day each strike; staying inside the same storm gives no extra blow. A storm can also drift onto a slow ship. [[rites|Favourable Passage]] still protects the ship on its outward leg.

**The [[gods|gods]]' hand.** Your [[kharis]] tilts each storm your ship meets. **Favorable:** a 15% chance that the storm does no harm — the notice says *Poseidon held back the waves*. **Wrathful:** a 15% chance that it takes one more hull, three instead of two. Indifferent and Suspicious change nothing. The **Kult (⛩)** drawer and Keryx `cult` show the chance before you sail. A ship that is spared still met the storm, so it can still earn the right to name it.

A storm standing on a city's fishing hex stops the fishing there until it moves on ([[fishing]]).

## Seeing and remembering storms {#storms-what-you-see}

You see a storm **only inside your own sight** ([[sight]]) — from your cities, your ships and your soldiers. There it is drawn as dark cloud, and hovering tells you which way it is drifting.

Once you have seen a storm, the map remembers it **where you last saw it**, as a faded cloud, and hovering says on which game day. That is not where it is now. If you see any part of that place and the storm is not there, it has moved on and the memory is dropped.

## Naming a storm {#storms-naming-a-storm}

The first Wanax whose ship a storm strikes **may [[naming-the-land|name it]]**, once — even if that blow sinks the ship. You are told in the storm notice. Click the storm on the map and give it a name (2 to 30 letters, digits, spaces, apostrophes and hyphens; no two storms share a name). The storm keeps the name wherever it drifts, and everyone who sees or remembers it will see the name. A name that is out of order can be changed or removed by the game's keepers; the one who gave it does not get a second try.

In Keryx: `storms` lists the storms in sight and the ones you remember, with their ids; `storms name <id> "<name>"` names one you were first to meet.

**Fishers stay at their grounds.** A storm makes them catch nothing while it covers their hex; it never sends them home or removes their placement. They resume fishing when it moves on. See [[fishing]].
