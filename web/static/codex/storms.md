A **storm** is three connected hexes of dark cloud on the Thalassa. There is about one for every 36 hexes of sea, and they never stop wandering. <!-- src: server/internal/combat/sea_storm_weather.go SeaHexesPerStorm -->

## How they move

Each day **one** of a storm's three hexes steps to a neighbouring sea hex, and the storm stays in one piece. So a storm crawls: slowly, and a little more each day in the direction it has been drifting, now and then turning. It never crosses land.

## What they do

A ship that **sails into a storm** takes two points of hull damage (of five) — once per storm, however many days it spends inside it; three storms sink a ship from full hull. At hull 0 she founders with everything aboard ([[sea]]). A ship beside a storm is safe. Ships follow the route they were given and cannot steer round a storm once they sail ([[sea]]), so look before you send them.

## What you see

You see a storm **only inside your own sight** ([[sight]]) — from your cities, your ships and your soldiers. There it is drawn as dark cloud, and hovering tells you which way it is drifting.

Once you have seen a storm, the map remembers it **where you last saw it**, as a faded cloud, and hovering says on which day. That is not where it is now. If you look at the place and it is gone, it has moved on and the memory is dropped.

## Naming a storm

The first Wanax whose ship a storm strikes **may name it**, once — even if that blow sinks the ship. You are told in the storm notice. Click the storm on the map and give it a name (2 to 30 letters, digits, spaces, apostrophes and hyphens; no two storms share a name). The storm keeps the name wherever it drifts, and everyone who sees or remembers it will see the name. A name that is out of order can be changed or removed by the game's keepers; the one who gave it does not get a second try.

In Keryx: `storms` lists the storms in sight and the ones you remember, with their ids; `storms name <id> "<name>"` names one you were first to meet.
