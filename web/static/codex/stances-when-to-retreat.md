When your men break off a battle is set in two places:

- **For the whole realm** — **War → Forces → When to retreat**, or `keryx retreat-default` ([[keryx]]). Choose **Hold to the last man** or **Retreat when it goes badly** (the default). The latter withdraws when 20% of the side’s starting strength remains. Every unit carries this into a battle it enters. It applies at once, since no one has to carry it anywhere, but only to battles entered from then on — a battle already under way keeps what its forces brought.
- **For one unit, in the battle it is fighting now** — the **this battle** selector on its card in **War → Forces** (it only appears while the unit is fighting), or `keryx unit retreat-order`. Choose the same two modes. It overrides the realm setting for that battle only. On a field unit it is an order, and travels by [[runners|Runner]].

In Keryx, use `--hold-to-last-man` or `--when-losing` with either command. Fixed loss percentages are no longer a choice.
