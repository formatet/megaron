**One citizen marker stands for one hundred people.** A city of a thousand gives you ten. Each citizen stands on **one hex, or in one building, producing one good**. That is the whole labour model.

## Placing citizens {#citizens-placing-people}

Open **City → Production → Catchment & workplaces**. You get a hex diagram: the centre hex labelled **City**, ringed by your [[catchment]]. Each ring hex shows its number and how many goods it can produce.

**Click a hex** and a row appears for each good it can produce, with:

- what it makes now and what **the next citizen would add**,
- occupancy as dots — filled for placed citizens, hollow for free places,
- or simply **full** when the hex is at its limit.

Each row has **+1**, **Fill** and **−1**. *Fill* keeps adding until the hex is full or your idle people run out. If a placement is refused, the reason appears in red under the rows; the hex stays selected so you can try another good.

**Click the centre City hex** to staff [[buildings]] — same rows, same buttons.

## The next citizen’s production {#citizens-the-one-number-that-matters}

The *next citizen would add* figure is all you need for a good decision. A citizen placed past a hex's limit produces nothing, which is why the row says **full** instead of letting you find out slowly. Every good has a limit on that hex; a relevant building adds four places and raises each worker's output.

## Free citizens and new population {#free-citizens}

Your first citizens are placed on the best food land when you [[founding|found]]. Afterwards, each time the population crosses another hundred, the new citizen remains **free to work** until you place it. Choose a hex or the central City hex and assign that citizen to a job. Keep enough citizens producing food as your city [[growth|grows]].

Idle citizens are wasted citizens. So are citizens producing something nobody uses — see [[surplus]].

On the map, a small marker shows the hexes where your people are at work.

[[fish]] feeds your people just as [[grain|grain]] does. A city living on stored food is still fed. Hunger begins only when the daily meal remains short after the [[food|feeding chain]] has been exhausted; see [[growth]].

One placeable **citizen represents 100 people**. Population counts people; City and Goods count these citizens. The food-workers estimate assumes the best grain and fish slots for the population. Moving workers may require rearranging the remaining food placements; the army draws from the same food stocks, as shown separately in the food balance.

### Food from fractional citizens {#free-citizens-2}

People outside whole groups of 100 automatically produce a fraction of the best food worker’s output: 42 people give 42 hundredths. They make grain or fish, whichever their best food hex produces most of, including its building bonus. This work is invisible and takes no place from your citizens. Nearby ground supplies its grain separately. City food details show these automatic contributions beside the placed citizens needed for meals. The count uses production rather than stored food; it is an estimate for the best food slots, not a recommendation to remove particular citizens.

[[temple]] workers use the same citizen pool as other jobs. Use `keryx staff temple +1` to assign one, `staff temple -1` to free one (with several temples, add `--god <god>`), and `roster` to see their assignments. See [[temples]].
