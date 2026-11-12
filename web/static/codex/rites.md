A rite is a request made at the temple of its god.

- Each prayer is said at its own god's [[temples|temple]]; level 1 is enough.
- [[kharis]] is a minimum you must hold, never spent; the offering is.
- Bring the traditional recipe, or a composed offering worth at least as much.
- An answered prayer makes **that temple** rest, and only while it is fully staffed.
- Twice the gift, twice the length and twice the temple's rest, up to five times. <!-- src: reference.json prayers -->

## Temples required by prayers {#rites-details}

[[temple]] per prayer: [[sea-daimons|Sea Daimons]] of Stillness: [[following-seas]], [[stout-hulls]], [[still-the-storm]], [[wake-the-storm]]. [[velchanos]]: [[green-shoots]], [[the-young-gods-arm]], [[builders-blessing]]. [[potnia]]: [[mistress-of-the-walls]], [[potnias-vision]], [[fruitful-season]], [[the-scapegoat]], [[the-walls-fall]]. [[asterion]]: [[swift-feet]], [[the-bulls-horns]], [[the-thread]], [[the-bird-oracle]].

## Standing and offerings for prayers {#prayer-requirements}

A prayer answers when you meet its standing, have its god's temple and can bring its offering. Waiting time, an active gift or an unused departure gift can still prevent a new prayer.

A lasting prayer offers the gift as 1, 2, 3 or 5 times the traditional one; the length and the temple's rest grow the same way, up to five times. Other goods go under **Offer other goods**; a smaller gift is refused without taking goods. In Keryx, `rite --offer good=amount` chooses the gift.

### Waiting between prayers {#prayer-requirements-2}

An answered prayer makes **that temple** rest, not the god or your other temples. The rest grows with the gift and shrinks with temple level: level 2 rests half as long as level 1, level 3 a third. **A temple recovers only while fully staffed**. Potnia's prayers share her temple's rest. An active gift, or one awaiting departure, cannot be renewed before it ends or is used.

## Prayer answers and active blessings {#rites-the-prayers}

`keryx rite --list` or **Kult → Prayers** shows what you can offer and why one is unavailable. The answer arrives as a dispatch naming the god, the prayer and what was granted; when the god marks a place, **⌖ Take me there** goes to it.

## The current Minoan prayers {#minoan-prayers}

Offerings, required kharis and rest are in the table in [[kharis]]. Durations below are for the traditional gift.

- **[[green-shoots|Green Shoots]]**: grain, [[oil|oil]] and [[wine|wine]] yield a quarter more for 25 game days; fish and the herd unchanged.
- **[[mistress-of-the-walls|Mistress of the Walls]]**: your garrison defending this city is a quarter stronger for 6 game days; no help to an attacker or to soldiers elsewhere. Shares Vision's rest.
- **[[potnias-vision|Potnia's Vision]]**: marks one source of [[copper|copper]], incense, silver or [[tin|tin]] you neither see nor remember, and unknown ground within two hexes, as grey memory. No living eyes.
- **[[the-young-gods-arm|The Young God's Arm]]**: named land troops roll a quarter more dice in [[battle]] for 6 game days, attacking or defending, wherever they stand.
- **[[stout-hulls|Stout Hulls]]**: each ship you name takes its next [[storms|storm]] unharmed, for 12 game days.

**Blessings with a target.** The Arm, Stout Hulls, Swift Feet, Following Seas, the Bull's Horns and the Thread ask who is blessed: **one named unit** (or [[runners|Runner]], caravan or ship), **everything of that kind supported by the city where you pray**, or **all of it in your realm**; the traditional offering ×1, ×2 or ×4. Keryx: `rite --scope unit|city|realm`, `--target <name or id>`. Only your own troops or ships of the right kind can be named. The blessing is read when needed, so it works on a unit already marching or at sea; a city or realm blessing also covers those that join later. The same target cannot be blessed twice at once.

**For ever (a fourth step).** The Arm (a quarter stronger in every battle), Following Seas (every voyage takes three quarters of the time) and Stout Hulls (hull 6, not 5) can be laid on **one named troop or ship, for good**, at **twenty times** the offering. Never a city, the realm, a Runner or a caravan. Keryx: `rite --scope permanent --target <name>`.

### Prayers laid on a city or the realm

You choose **the city where you pray** (twice the offering) or **the whole realm** (four times); Keryx `rite --scope city|realm`.

- **[[fruitful-season|Fruitful Season]]**: people and herds grow half as fast again for 20 game days.
- **[[builders-blessing|The Builder's Blessing]]**: each building and ship under construction, and those begun meanwhile, takes three quarters of the time, for 12 game days.
- **[[the-scapegoat|The Scapegoat]]**, realm only (so 12 livestock): a goat bears the next divine punishment within 30 game days, so nothing is lost, and [[gods|the gods]]' mercy holds as after any punishment.

### Prayers of battle and of the road

- **[[the-bulls-horns|The Bull's Horns]]**: named land troops strike half as hard again in the **first round** of a battle, for 12 game days; stacks with the Arm in that round. No step for ever.
- **[[the-thread|The Thread]]**: named land [[transfers|caravans]] can be taken only by a sentry within one hex, not two, for 12 game days, also on the road. Runners and ships are not covered.
- **[[the-bird-oracle|The Bird Oracle]]** (30 [[kharis]] or more): name a city or place on your map and for 1 game day you see live everything within 2 hexes, armies, fleets and caravans with strength. Afterwards it is grey memory. The one watched is told nothing.

### A prayer against another Wanax

- **[[the-walls-fall|The Walls Fall]]**: name a city of another Wanax that is on your map and whose wall you remember. Three game days later it loses one wall level. You learn the result by seeing the city again. The other Wanax is told when the wall falls, warned beforehand if they stand well with the gods, never by whom. A richer gift does no more. See [[wrath-over-the-walls]].

### Prayers of speed {#minoan-prayers-2}

- **[[swift-feet|Swift Feet]]**: named Runners and land caravans take a quarter less time over what is left of their road, for 12 game days.
- **[[following-seas|Following Seas]]**: named ships take a quarter less time over what is left of their voyage, for 12 game days. A journey is shortened once, however many prayers are said.

Ares' Fury (Akhaier) works like Mistress of the Walls. Green Shoots requires 30 kharis; the rest at least 5.

### Gifts awaiting departure {#minoan-prayers-3}

A departure gift stays in the city where you offered until an eligible journey physically leaves. It is used once, and cannot change a journey underway or bless a return leg. **Awaiting next departure** shows beside the prayer until then. `rite --prayer <id>` chooses a prayer; omitting it uses the culture's default (Mistress of the Walls for Minoans).

## The value of an offering {#offerings}

The gods keep a fixed reckoning: what one unit of each good is worth as an offering, before a particular god's preferences.

| Good | Worth |
|---|---|
| [[grain]] | 1 |
| [[fish]], [[livestock]], [[silver]], [[timber]] | 2 |
| [[cedar]], [[copper]], [[oil]], [[stone]], [[tin]], [[wine]] | 3 |
| [[incense]] | 12 |
| [[bronze]] | 33 |

These are offering values, not trade prices. A good's worth is multiplied by the god's taste for it (above 1 the god values it more, below 1 less; unlisted goods count at their worth). The traditional recipe always suffices.

<!-- favours -->
| Prayer | God | Favours (dearest first) |
|---|---|---|
| Following Seas | The Sea Daimons of Stillness | no preference: every good at its worth |
| Fruitful Season | Potnia | incense ×2, wine ×2, oil ×1.6, grain ×1.3, livestock ×1.3, fish ×0.9, copper ×0.5, tin ×0.5, timber ×0.4, stone ×0.3 |
| Green Shoots | Velchanos | incense ×2, wine ×2, oil ×1.6, grain ×1.3, livestock ×1.3, fish ×0.9, copper ×0.5, tin ×0.5, timber ×0.4, stone ×0.3 |
| Mistress of the Walls | Potnia | livestock ×2, wine ×1.7, bronze ×1.5, incense ×1.5, grain ×1, fish ×0.4, stone ×0.4 |
| Potnia's Vision | Potnia | fish ×2.2, incense ×2.1, oil ×1.5, silver ×1.4, bronze ×1.3, grain ×0.6, stone ×0.4 |
| Still the Storm | The Sea Daimons of Stillness | fish ×2.2, livestock ×2, wine ×1.7, bronze ×1.5, incense ×1.5, grain ×1, stone ×0.4 |
| Stout Hulls | The Sea Daimons of Stillness | fish ×2.2, livestock ×2, wine ×1.7, bronze ×1.5, incense ×1.5, grain ×1, stone ×0.4 |
| Swift Feet | Asterion | no preference: every good at its worth |
| The Bird Oracle | Asterion | livestock ×2, wine ×1.7, bronze ×1.5, incense ×1.5, grain ×1, fish ×0.4, stone ×0.4 |
| The Builder's Blessing | Velchanos | no preference: every good at its worth |
| The Bull's Horns | Asterion | livestock ×2, wine ×1.7, bronze ×1.5, incense ×1.5, grain ×1, fish ×0.4, stone ×0.4 |
| The Scapegoat | Potnia | incense ×2, wine ×2, oil ×1.6, grain ×1.3, livestock ×1.3, fish ×0.9, copper ×0.5, tin ×0.5, timber ×0.4, stone ×0.3 |
| The Thread | Asterion | no preference: every good at its worth |
| The Walls Fall | Potnia | livestock ×2, wine ×1.7, bronze ×1.5, incense ×1.5, grain ×1, fish ×0.4, stone ×0.4 |
| The Young God's Arm | Velchanos | livestock ×2, wine ×1.7, bronze ×1.5, incense ×1.5, grain ×1, fish ×0.4, stone ×0.4 |
| Wake the Storm | The Sea Daimons of Stillness | fish ×2.2, livestock ×2, wine ×1.7, bronze ×1.5, incense ×1.5, grain ×1, stone ×0.4 |
<!-- /favours -->

Any good may be offered. **Oil and wine** are offerings like any other — oil is also food, so do not give up what a hungry city needs. **Incense** is a valuable gift ([[incense]]). For everyday prayers it can help meet the offering requirement; excess value does not change the answer, but a lasting prayer lasts longer and the temple rests longer.
