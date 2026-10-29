A city grows when it has a **surplus of food** — when it produces more food each game day than its people eat. Growing costs no food: the surplus stays in the same city [[grain|grain]], [[fish|fish]] and [[oil|oil]] stocks that feed your people.

- **Surplus, not store.** What counts is the daily balance — grain, fish and oil produced, minus what the people eat — shown as the food *net* on the city. A positive net grows the city; zero or below holds it steady, even with full stores.

- **Rate.** A fed city with a positive net grows by **0.5% of its people per game day** (at least one person), before the variety bonus below. A city of 1 000 gains about 5 a game day.

- **Surplus size.** A bigger surplus fills your stores faster; it does not itself raise the growth rate. <!-- src: server/internal/economy/population_growth.go PopulationGrowthRatePerTick -->

## Growth from food surplus {#growth-details}

- **Variety.** Any of the food goods feeds people. Growth gains 10% per extra kind that forms at least **10% of the meal** eaten that game day, up to the existing variety ceiling: two substantial kinds give a 1.1 multiplier and four give 1.3. [[livestock]] counts only if eaten as a real share of emergency food. A good merely sitting in store, or eaten only in crumbs, gives no variety bonus.

- **Size.** The city's walls set its ceiling: 2 000 people with no wall, then 5 000, 10 000 and 20 000 for a Palisade, [[stone]] [[walls]] and [[bronze|Bronze]] Wall ([[buildings]]). Growth runs at its full rate right up to the ceiling and then stops; a city at its ceiling never shrinks because of it. The city screen says "Walls hold N", and `keryx status` says it too.

### When population growth stops {#growth-details-2}

In short: a city grows only while it makes more food than it eats; it holds steady when it does not; and it shrinks when it goes hungry. The city screen and `keryx status` say which of the three it is, and the food net behind it, for example "not growing: food net -1.7 per game day (needs more than 0)".

More people eat more, so a growing city eats into its own surplus until the balance reaches zero and growth stops — unless you put the new citizens to work on food.

Each time the population crosses another hundred, a new [[citizens|citizen]] becomes **free to work**. Choose its job yourself.

More people means more workers, more recruits and more mouths. See [[food]].

## The next free citizen {#growth-next-citizen}

Under **Citizens**, the City screen shows ten solid steps toward your next
citizen and the exact progress out of 100 people. For example, **64 / 100** means
36 more people are needed for the next citizen you can place. The estimated wait
uses the city's current growth, including food variety and the room inside its
walls. Game days appear with a real-time estimate from your world's clock;
when one game day or less remains, it says **next game day**.

The estimate changes with your city. A city with no growth says **not growing**;
at its population ceiling, **the walls are full**; during hunger, **shrinking**.
Keryx `city` and `status` show the same progress and explanation.
