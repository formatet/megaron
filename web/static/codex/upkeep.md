Soldiers eat **food: grain, fish, oil and wine**, shared in proportion to the stocks available. Professionals also need **silver pay** every day.

## Pay — silver

**Spearmen draw 0 silver per day**, in garrison and in the field. Elite infantry, chariots and ships still draw pay in [[silver]] every day. If you cannot pay, you are warned first; if it goes on, **men desert**, and a unit that loses everyone is gone. Recruit professionals only when you can keep paying them. A spearman cohort still costs 12 grain, 20 silver and 100 people to recruit.

An empty ship **Laid up** in its own Harbour costs **0 silver and 0 ship food**. Its crew has returned to the city's population and eats as civilians. **Fit out** drafts that crew back immediately; ordinary ship food and silver upkeep resume while the ship spends three days preparing. See [[sea]].

## Food

Soldiers use the same four [[food|food goods]] as the people. Livestock remains emergency food for civilians, not an army ration. A full spearman cohort uses **0.5 food per day in garrison**, or **1 food in the field**. A unit in garrison eats from its city. **A unit in the field eats double**, and there is nothing to forage — so a long march, a siege line or a posted watch is a real cost ([[stances]]). <!-- src: server/internal/combat/upkeep.go upkeepFieldGrainFactor -->

**Rowers eat like soldiers.** A ship feeds its actual surviving crew: each rower uses the same food as a spearman. A full Galley (20 rowers) uses **0.10 food per day in port** or **0.20 on a mission**; a War Galley (50) uses **0.25 / 0.50**, and an Emporos (10) **0.05 / 0.10**. Fewer surviving rowers eat less. Garrisoned ships, repairs and fitting out use the port ration. Marching, posted and freighting ships use the field ration, including a freight route's wait between legs. A laid-up hull has no ship ration because its crew has returned to the city. Ship pay is unchanged.

A unit that runs out of food **starves**: it loses men each day, and can starve to nothing. A ship whose crew runs short turns for home by itself, and says so.

The Notifications archive marks when a city’s garrison starts losing men to hunger and when it stops. A recovery means that loss has stopped; it does not promise food for every soldier who may arrive later. Produce more food or send food by [[transfers|transfer]].

## Before you settle

Your [[horde|Nomadic Host]]'s spearman escort draws no silver. The silver reserve you carry goes into the city when you settle; it does not limit how long the escort can travel.

## The rule of thumb

Your army is only as large as your economy can feed and pay. Build [[food]] surplus before you build cohorts, and [[silver]] income before expanding your professional forces ([[units]]).
