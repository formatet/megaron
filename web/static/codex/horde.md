You do not begin with a city. You begin as a **Nomadic Host** — a whole people on the move, standing as one token on the map. There is no separate founding screen: the map is the game from the first second.

Find it (the map centres on you) and **left-click the hex it stands on**. The inspect panel opens, headed *Nomadic Host*.

## The Host’s people and escort {#horde-what-the-host-is}

- **A thousand people**, and they are the city you will found — its starting population, not soldiers. <!-- src: api/handlers/nomadic_host.go nomadicHostPopulation -->
- **Two cohorts of [[spearmen|spearmen]] escort you.** They live on [[grain|grain]] and draw no [[silver|silver]] pay, just like settled spearmen.
- **The Host itself eats nothing.** A people on the move carries its own bread. The grain in the store is a dowry that goes into your city's stores the game day you found.
- **The Host cannot fight.** The escort can.
- **It sees two hexes** (further across open water or towards mountains) — exactly the ground a settlement would work. Wherever the Host stands, it can see the whole [[catchment]] it would get by stopping there.

## Moving the Nomadic Host {#horde-walking}

**Right-click any hex** to open the order menu, pick the Host and press **March →** — or left-click a hex and use **March here →** in the inspect panel. The Host moves at 90% of Spearmen's speed and reveals the map as it goes.

Unlike everything else you will ever command, **the Host obeys instantly**: you are walking with it. See [[runners]] for what comes after.

## Inspecting the Host’s current ground {#horde-where-you-are}

The Host's marker, sight and founding place follow the Province it physically occupies, including during a march. Its hover and panel say **Marching**, **Waiting** (with the reason this ground cannot be settled), or **Can found here**. Click the marker's current hex to open the Host panel. Keryx `founding status` and `status` show the same position and state.

You can found on valid ground while the Host is marching. The city rises where the Host stands when you confirm, and the Host disappears. If it has moved from the place you confirmed, the order is refused; reopen its panel and read the new site before founding.

## The Host’s founding reserves {#horde-reserves-for-founding}

Before founding, the Host lives on subsistence and its reserves do not drain. A new Host carries **120 silver**; a Host rising after losing every city starts with **60 silver**. Its grain and silver go into your city when you settle. Your [[welcome|opening dispatch]] records what your Host carries. Choose your site for its food, resources and position.

When you have found your place: [[founding]].
