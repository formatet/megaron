A rite is a request made at the temple of its god.

A **rite** is a request to [[gods|the gods]]. Open **Kult → Prayers**.

Each prayer is said at its own god's [[temples|temple]], so a city needs that temple standing: [[following-seas]], [[stout-hulls]], [[still-the-storm]] and [[wake-the-storm]] need a Temple of the [[sea-daimons|Sea Daimons]] of Stillness, [[green-shoots]], [[the-young-gods-arm]] and [[builders-blessing]] a Temple of [[velchanos]], [[mistress-of-the-walls]], [[potnias-vision]], [[fruitful-season]] and [[the-scapegoat]] a [[temple]] of [[potnia]], [[swift-feet]] a Temple of [[asterion]]. Level 1 is enough. Without it the prayer shows "no Temple of …".

## Temples required by prayers {#rites-details}

Each prayer row shows its temple, **Required kharis: N** (the minimum standing it needs, never spent), its effect and its **Offering** (the traditional recipe). Press **Offer**: a row opens with a field for each good, filled with the prayer's recipe. Bring it as it stands, or change the numbers first.

## Standing and offerings for prayers {#prayer-requirements}

**[[kharis]] is never spent on a prayer.** Each of the ten Minoan everyday prayers answers when you meet its standing requirement, have its god's temple, and can bring its offering. Waiting time, an active gift or an unused departure gift can still prevent a new prayer.

Bring the traditional recipe or a composed offering worth at least as much to that god. If you lack the traditional gift, **Compose offering** opens empty fields so you can choose other goods you hold. Choose some goods before bringing the gift. `keryx rite --list` marks this as **compose**; use `rite --offer good=amount` to choose the gift. In an interactive terminal, Keryx shows your chosen goods and exact amounts before asking **Cast?**; answering no leaves the gift untouched. A smaller gift is refused without taking goods. Bringing more does not change whether the god answers, but a larger gift makes a lasting prayer last longer (up to five times as long) and makes the temple rest longer. For these prayers, `--offer-multiplier` is ignored; omitting a composed offering always brings the traditional recipe.

### Waiting between prayers {#prayer-requirements-2}

An answered prayer makes **that temple** rest — not the god, and not your other temples. Another temple of the same god in another city can still pray meanwhile. The rest grows with the size of the gift and shrinks with the temple's level: a level 2 temple rests half as long as a level 1 temple for the same gift, a level 3 temple a third. **A temple recovers only while it is fully staffed**; if it lacks servants its rest stands still, and **Kult** and `keryx rite --list` say so ("paused: 2 of 4 servants"). Potnia's two prayers share her temple's rest. **Kult** and `keryx rite --list` show the rest, an active gift or a gift awaiting departure. Missing temples and insufficient kharis are named as separate requirements. An active gift cannot be renewed before it ends. A gift awaiting departure cannot be renewed before it is used.

Older prayers outside the current Minoan catalogue retain their uncertain answer and offering multiplier. Their offering is spent even on a silent answer, but kharis is never spent. Old recorded outcomes remain as they happened.

## Prayer answers and active blessings {#rites-the-prayers}

The current Minoan prayers are **Following Seas**, **Fruitful Season**, **Green Shoots**, **Mistress of the Walls**, **Potnia's Vision**, **Stout Hulls**, **Swift Feet**, **The Builder's Blessing**, **The Scapegoat** and **The Young God's Arm**, described below. `keryx rite --list` or **Kult → Prayers** shows the prayers your culture can offer and why one is unavailable. Current kharis is shown with a decimal; eligibility uses the exact pool value.

The answer or a refusal appears in the Kult drawer after you offer; it stays beside the prayers. An answer also arrives as a dispatch naming the god, the prayer and what was granted. When the god marks a place, **⌖ Take me there** goes to it. In Keryx it is listed by `keryx notifications`.

An ongoing **blessed harvest** appears beside Food in the city drawer and in
`keryx status` and `keryx city`, with its yield bonus, affected goods and game days left.
The row disappears when the blessing ends; [[fish|fish]] receives no harvest bonus.

## The current Minoan prayers {#minoan-prayers}

- **Green Shoots** — offer 15 grain + 2 silver. Velchanos raises grain, [[oil|oil]] and [[wine|wine]] yield by a quarter for 25 game days with the traditional gift; a larger gift lasts longer. Fish and the herd are unchanged. His temple rests 25 game days after answering at level 1.
- **Mistress of the Walls** — offer 8 grain + 4 [[timber|timber]]. Potnia strengthens your own garrison defending this city by a quarter for 6 game days with the traditional gift; a larger gift lasts longer. It helps neither an attacking army nor soldiers away from that city. It shares Vision's rest.
- **Potnia's Vision** — offer 10 [[grain|grain]] + 3 [[silver|silver]]. Potnia marks one source of [[copper|copper]], incense, silver or [[tin|tin]] you neither see nor remember, and unknown ground within two hexes, as grey memory. It creates no living eyes and shows no moving activity. Her temple rests 24 game days after answering at level 1.

- **The Young God's Arm** — offer 6 [[livestock|livestock]] + 4 [[wine|wine]]. Velchanos makes the land troops you name roll a quarter more dice in [[battle]] for 6 game days with the traditional gift, attacking or defending, wherever they stand. It is a **blessing with a target**, see below. His temple rests 12 game days after answering at level 1.
- **Stout Hulls** — offer 8 fish + 4 [[oil|oil]]. The Sea Daimons let each ship you name take its next [[storms|storm]] unharmed, for 12 game days with the traditional gift. It is a **blessing with a target**, see below. Their temple rests 12 game days after answering at level 1.

**Blessings with a target.** These four prayers ask who is blessed: **one named unit** (or [[runners|Runner]], caravan or ship) of yours, **everything of that kind supported by the city where you pray**, or **all of it in your realm**. The wider the target, the dearer the gift: the traditional offering ×1, ×2 or ×4. A larger gift than that lasts longer, as for any lasting prayer. In **Kult → Prayers** the offer row has the three choices, the list of your troops or ships, and the gift for each; in Keryx use `rite --scope unit|city|realm` and, for one unit, `--target <name or id>`; `rite --list` shows what you can name. Only your own, living troops or ships of the right kind can be named (troops for the Arm, ships for Stout Hulls and Following Seas, Runners and caravans for Swift Feet). The blessing is read when it is needed, wherever the blessed troops or ship are, so it works for a unit already marching or at sea. The same target cannot be blessed twice at once, but two different troops or ships can each be blessed, once the temple has rested.

**For ever (a fourth step).** Three of these prayers can also be laid on **one named troop or ship, for good**, at **twenty times** the traditional offering: the Arm (fights a quarter stronger in every battle), Following Seas (every voyage of that ship takes three quarters of the time) and Stout Hulls (one plank more, so her hull is 6 and not 5, repaired back to 6). It is never a city or the realm, and never a Runner or caravan, which make one journey each (Swift Feet has no such step). The same blessing cannot be said twice over the same troop or ship. The temple rests as after the ordinary prayer. Choose *For ever* in **Kult → Prayers**, or use `rite --scope permanent --target <name>`. Your force list shows what is laid on each troop or ship; other Wanaxer do not see it.

### Prayers laid on a city or the realm

These three name no unit; you choose **the city where you pray** (twice the traditional offering) or **the whole realm** (four times). In **Kult → Prayers** the offer row has just those choices; in Keryx use `rite --scope city|realm`.

- **Fruitful Season** — offer 4 livestock + 4 grain to Potnia. People and herds grow half as fast again for 20 game days with the traditional gift, see [[fruitful-season]].
- **The Builder's Blessing** — offer 3 timber + 2 wine to Velchanos. What is left of each building and ship under construction takes three quarters of the time, once for each, and so do those begun while it lasts, for 12 game days with the traditional gift, see [[builders-blessing]].
- **The Scapegoat** — offer 3 livestock to Potnia, for the realm only (so 12 livestock). A goat bears the next divine punishment within 30 game days, so nothing is lost; the gods' mercy holds as after any punishment, see [[the-scapegoat]]. A notice tells you when it has done so.

### Prayers of speed {#minoan-prayers-2}

- **Swift Feet** — offer 3 silver + 4 timber to Asterion. Runners and land [[transfers|caravans]] you name take a quarter less time over what is left of their road, also when already under way, for 12 game days with the traditional gift. A **blessing with a target**, see above; the city and realm steps also hold for journeys that start while the prayer lasts. Asterion's temple rests 12 game days after answering at level 1.
- **Following Seas** — offer 6 fish + 3 silver to The Sea Daimons of Stillness. Ships you name take a quarter less time over what is left of their voyage, also when already at sea, for 12 game days with the traditional gift. A **blessing with a target**, see above. A journey is shortened once, however many prayers are said. Their temple rests 12 game days after answering at level 1.

*Retired:* **The Thread Shows a Path** and **Favourable Passage** (next-departure prayers) can no longer be offered; a departure prayer already waiting is still honoured when its journey leaves.

Other cultures' war prayer (for example Ares' Fury for the Akhaier) works the same way as the Walls: your own garrison defending this city fights a quarter stronger for 6 game days with the traditional gift, and it cannot be offered again while it is active. It does not help an attacking army.

Green Shoots requires 30 kharis; Vision, [[walls|Walls]], the two prayers of speed and the two blessings require at least 5. The temple's ordinary daily offering is separate.

### Gifts awaiting departure {#minoan-prayers-3}

A departure gift stays in the city where you offered until an eligible journey physically leaves. It is used once. It cannot follow the city to a new owner, change a journey already underway, or bless a return leg. **Awaiting next departure** appears beside the prayer while the gift is unused; the god's waiting time is shown separately. Other ongoing gifts show their remaining game days.

In Keryx, `rite --list` shows your prayers, their effects, offerings, active game days, pending departure gifts of the retired prayers and each temple's rest. `rite --prayer <id>` chooses one. Omitting the ID uses the prayer marked as the culture's default — Mistress of the Walls for Minoans.

## The value of an offering {#offerings}

The gods keep a fixed reckoning: what one unit of each good is worth as an offering, before a particular god's preferences.

| Good | Worth |
|---|---|
| [[grain]] | 1 |
| [[fish]], [[livestock]], [[silver]], [[timber]] | 2 |
| [[cedar]], [[copper]], [[oil]], [[stone]], [[tin]], [[wine]] | 3 |
| [[incense]] | 12 |
| [[bronze]] | 33 |

`keryx cult` and `keryx rite --list` print the same figures; the web prayer row shows what each god **Favours**.

These are offering values. Wanaxes agree their own trade prices. You may compose an offering in the Offer row on the web, or in Keryx with `rite --offer good=amount,...`. Each god has a taste: the Offer row's **Favours:** line lists the goods this god loves most, dearest first. The traditional recipe always suffices.

A good's worth is multiplied by the god's taste for it. A taste above 1 means the god values that good more than its worth, below 1 less. Goods not listed count at their worth.

<!-- favours -->
| Prayer | God | Favours (dearest first) |
|---|---|---|
| Following Seas | The Sea Daimons of Stillness | no preference: every good at its worth |
| Fruitful Season | Potnia | incense ×2, wine ×2, oil ×1.6, grain ×1.3, livestock ×1.3, fish ×0.9, copper ×0.5, tin ×0.5, timber ×0.4, stone ×0.3 |
| Green Shoots | Velchanos | incense ×2, wine ×2, oil ×1.6, grain ×1.3, livestock ×1.3, fish ×0.9, copper ×0.5, tin ×0.5, timber ×0.4, stone ×0.3 |
| Mistress of the Walls | Potnia | livestock ×2, wine ×1.7, horses ×1.6, bronze ×1.5, incense ×1.5, grain ×1, pottery ×0.5, fish ×0.4, stone ×0.4 |
| Potnia's Vision | Potnia | fish ×2.2, incense ×2.1, purple ×2, oil ×1.5, silver ×1.4, bronze ×1.3, pottery ×1.1, grain ×0.6, stone ×0.4 |
| Still the Storm | The Sea Daimons of Stillness | fish ×2.2, livestock ×2, wine ×1.7, horses ×1.6, bronze ×1.5, incense ×1.5, grain ×1, pottery ×0.5, stone ×0.4 |
| Stout Hulls | The Sea Daimons of Stillness | fish ×2.2, livestock ×2, wine ×1.7, horses ×1.6, bronze ×1.5, incense ×1.5, grain ×1, pottery ×0.5, stone ×0.4 |
| Swift Feet | Asterion | no preference: every good at its worth |
| The Builder's Blessing | Velchanos | no preference: every good at its worth |
| The Scapegoat | Potnia | incense ×2, wine ×2, oil ×1.6, grain ×1.3, livestock ×1.3, fish ×0.9, copper ×0.5, tin ×0.5, timber ×0.4, stone ×0.3 |
| The Young God's Arm | Velchanos | livestock ×2, wine ×1.7, horses ×1.6, bronze ×1.5, incense ×1.5, grain ×1, pottery ×0.5, fish ×0.4, stone ×0.4 |
| Wake the Storm | The Sea Daimons of Stillness | fish ×2.2, livestock ×2, wine ×1.7, horses ×1.6, bronze ×1.5, incense ×1.5, grain ×1, pottery ×0.5, stone ×0.4 |
<!-- /favours -->

Any good may be offered. **Oil and wine** are offerings like any other — oil is also food, so do not give up what a hungry city needs. **Incense** is a valuable gift ([[incense]]). For everyday prayers it can help meet the offering requirement; excess value does not change the answer, but a lasting prayer lasts longer and the temple rests longer.
