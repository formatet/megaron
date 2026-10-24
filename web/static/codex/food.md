Your people eat **grain, fish, oil and wine** every game day. One unit of each supplies one unit of food. The meal is shared in proportion to what is available, including the game day’s production: a city with 100 grain and 900 fish takes one tenth of its meal as grain and nine tenths as fish. No food good is used up first. If these four cannot feed the people, livestock remains their emergency food: each animal supplies 3 food. Armies, ships and caravans use the four food goods, not livestock.

## When there is not enough

If the available food and emergency livestock run out and the game day's need is still unmet:

- the settlement **loses part of its population that game day**, and keeps losing while the shortfall lasts;
- it shrinks until what its land yields can feed those who are left — the ground right around a city always feeds about a hundred;
- below **50 people** — starved there, or emptied by drafting soldiers — **the settlement collapses and is gone.**

You get an urgent **CivilianStarvationStarted** notification when hunger is first observed. It records the game day, city and how much of that game day's meal was missing, in food. The latest start stays above ordinary history in Notifications until that city's food supply has recovered steadily. You get **CivilianStarvationEnded** after a run of fully fed game days; the old start then returns to ordinary history. Hunger returning during that recovery window belongs to the same warning. A new spell after recovery starts a new pair of notices, rather than repeating the warning every game day.

Civilian meals and [[upkeep|soldiers’ food]] draw from the same stocks, but are served separately: a city's people may be fed while its garrison is hungry, or the other way round. Each warning clears independently. If a city leaves your rule or ceases to be active, its warning closes without claiming its people recovered.

Older **FoodShortfall** messages remain in history. The world's daily food audit still records every shortfall; new inbox messages only mark the transitions. Check **City → Food details** for the current situation and act before the city keeps shrinking.

## The herd and the pen

The herd breeds itself — no herder is needed. Every ten animals give one calf every **5 game days** when the city has grain to feed it (**1 grain per calf**), and every **20 game days** on pasture alone. If the grain does not reach every calf, the fed ones come on the 5-game day beat and the rest wait for the 20-game day beat. A city can only keep so many animals: the **herd cap** is 50, and every level of a **Pen** ([[buildings]]) adds 100. **Status** and **City → Food** show it as *Herd 50 / 50*, with the next calves ("4 calves in 3 game days (4 grain)"). Each animal is worth **3 food**. Animals **above** the cap are eaten **first** at the game day's meal — fresh meat does not keep — and the four goods share whatever hunger is left. If animals are still above the cap after the meal, the pen cannot hold them and they are **lost**. A bigger Pen keeps more of them. Animals under the cap stay in the herd and are eaten only if the people would otherwise starve.

## What to do about it

- Put more [[citizens]] on grain or fish hexes. Check the *next citizen would add* figure.
- Build or level a **Farm** ([[buildings]]) — it raises how many people a grain hex can hold.
- Bring food in from another of your cities by [[transfers|transfer]] or a [[routes|standing order]].
- Buy it ([[trade]]).
- Shrink what eats: soldiers are drafted from your people and eat like them ([[upkeep]]).

Your city has food stocks: the grain, fish, oil and wine in its [[goods|stores]]. **Food for N game days** shows those stocks at current consumption, without counting future production. **City → More → Food details** shows production, the last meal by good and army food separately. Different goods forming a real share of the meal improve [[growth|population growth]]; merely storing another food does not. Stored food buys time during a shortfall. A city under [[sieges|siege]] starves because its fields are cut off, not because it produces too little.

Growth is covered in [[growth]].
