A **storm** is three connected hexes of dark cloud on the Thalassa. There is about one for every 36 hexes of sea, and they never stop wandering. <!-- src: server/internal/combat/sea_storm_weather.go SeaHexesPerStorm -->

## How they move

Each day **one** of a storm's three hexes steps to a neighbouring sea hex, and the storm stays in one piece. So a storm crawls: slowly, and a little more each day in the direction it has been drifting, now and then turning. It never crosses land.

## What they do

A ship that **sails into a storm** takes two points of hull damage (of five) — once per storm, however many days it spends inside it; three storms sink a ship from full hull. at hull 0 she founders with everything aboard ([[sea]]). A ship beside a storm is safe. Ships follow the route they were given and cannot steer round a storm once they sail ([[sea]]), so look before you send them.

## What you see

You see a storm **only inside your own sight** ([[sight]]) — from your cities, your ships and your soldiers. There it is drawn as dark cloud, and hovering tells you which way it is drifting.

Once you have seen a storm, the map remembers it **where you last saw it**, as a faded cloud, and hovering says on which day. That is not where it is now. If you look at the place and it is gone, it has moved on and the memory is dropped.

In Keryx: `storms` lists the storms in sight and the ones you remember.
