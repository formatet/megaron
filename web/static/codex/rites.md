A rite is a request made at the temple of its god.

A **rite** is a request to [[gods|the gods]]. Open **Kult → Prayers**.

Each prayer is said at its own god's [[temples|temple]], so a city needs that temple standing: [[potnias-vision]] and [[mistress-of-the-walls]] need a [[temple]] of [[potnia]], [[green-shoots]] a Temple of [[velchanos]], [[the-thread-shows-a-path]] a Temple of [[asterion]], [[favourable-passage]] a Temple of the [[sea-daimons|Sea Daimons]] of Stillness. Level 1 is enough. Without it the prayer shows "no Temple of …".

## Temples required by prayers {#rites-details}

Each prayer row shows its temple, **Required kharis: N** (the minimum standing it needs, never spent), its effect and its **Offering** (the traditional recipe). Press **Offer**: a row opens with a field for each good, filled with the prayer's recipe. Bring it as it stands, or change the numbers first.

## Standing and offerings for prayers {#prayer-requirements}

**[[kharis]] is never spent on a prayer.** Each of the five Minoan everyday prayers answers when you meet its standing requirement, have its god's temple, and can bring its offering. Waiting time, an active gift or an unused departure gift can still prevent a new prayer.

Bring the traditional recipe or a composed offering worth at least as much to that god. If you lack the traditional gift, **Compose offering** opens empty fields so you can choose other goods you hold. Choose some goods before bringing the gift. `keryx rite --list` marks this as **compose**; use `rite --offer good=amount` to choose the gift. In an interactive terminal, Keryx shows your chosen goods and exact amounts before asking **Cast?**; answering no leaves the gift untouched. A smaller gift is refused without taking goods. Bringing more does not change whether the god answers, but a larger gift makes a lasting prayer last longer (up to five times as long) and makes the temple rest longer. For these prayers, `--offer-multiplier` is ignored; omitting a composed offering always brings the traditional recipe.

### Waiting between prayers {#prayer-requirements-2}

An answered prayer makes **that temple** rest — not the god, and not your other temples. Another temple of the same god in another city can still pray meanwhile. The rest grows with the size of the gift and shrinks with the temple's level: a level 2 temple rests half as long as a level 1 temple for the same gift, a level 3 temple a third. **A temple recovers only while it is fully staffed**; if it lacks servants its rest stands still, and **Kult** and `keryx rite --list` say so ("paused: 2 of 4 servants"). Potnia's two prayers share her temple's rest. **Kult** and `keryx rite --list` show the rest, an active gift or a gift awaiting departure. Missing temples and insufficient kharis are named as separate requirements. An active gift cannot be renewed before it ends. A gift awaiting departure cannot be renewed before it is used.

Older prayers outside the current Minoan catalogue retain their uncertain answer and offering multiplier. Their offering is spent even on a silent answer, but kharis is never spent. Old recorded outcomes remain as they happened.

## Prayer answers and active blessings {#rites-the-prayers}

The current Minoan prayers are **Potnia's Vision**, **Mistress of the Walls**, **Green Shoots**, **The Thread Shows a Path** and **Favourable Passage**, described below. `keryx rite --list` or **Kult → Prayers** shows the prayers your culture can offer and why one is unavailable. Current kharis is shown with a decimal; eligibility uses the exact pool value.

The answer or a refusal appears in the Kult drawer after you offer; it stays beside the prayers. An answer also arrives as a dispatch naming the god, the prayer and what was granted. When the god marks a place, **⌖ Take me there** goes to it. In Keryx it is listed by `keryx notifications`.

An ongoing **blessed harvest** appears beside Food in the city drawer and in
`keryx status` and `keryx city`, with its yield bonus, affected goods and game days left.
The row disappears when the blessing ends; [[fish|fish]] receives no harvest bonus.

## The current Minoan prayers {#minoan-prayers}

- **Potnia's Vision** — offer 10 [[grain|grain]] + 3 [[silver|silver]]. Potnia marks one source of [[copper|copper]], [[tin|tin]], silver or incense you neither see nor remember, and unknown ground within two hexes, as grey memory. It creates no living eyes and shows no moving activity. Her temple rests 24 game days after answering at level 1.
- **Mistress of the Walls** — offer 8 grain + 4 [[timber|timber]]. Potnia strengthens your own garrison defending this city by a quarter for 6 game days with the traditional gift; a larger gift lasts longer. It helps neither an attacking army nor soldiers away from that city. It shares Vision's rest.
- **Green Shoots** — offer 15 grain + 2 silver. Velchanos raises grain, [[oil|oil]] and [[wine|wine]] yield by a quarter for 25 game days with the traditional gift; a larger gift lasts longer. Fish and the herd are unchanged. His temple rests 25 game days after answering at level 1.

### Departure prayers {#minoan-prayers-2}

- **The Thread Shows a Path** — offer 4 timber + 3 silver to Asterion. The next land [[transfers|caravan]] or [[runners|Runner]] leaving this city takes a quarter less time on its outward leg. A land caravan can be intercepted only within one hex of an enemy army, instead of two. Runners remain sacred. Asterion's temple rests 12 game days after answering at level 1.
- **Favourable Passage** — offer 6 fish + 3 silver to The Sea Daimons of Stillness. The next ship leaving this city takes a quarter less time and suffers no [[storms|storm]] damage on its outward leg. A Runner aboard travels at that ship's actual speed. Their temple rests 12 game days after answering at level 1.

Vision, [[walls|Walls]] and the two departure prayers require at least 5 kharis; Green Shoots requires 30. The temple's ordinary daily offering is separate.

### Gifts awaiting departure {#minoan-prayers-3}

A departure gift stays in the city where you offered until an eligible journey physically leaves. It is used once. It cannot follow the city to a new owner, change a journey already underway, or bless a return leg. **Awaiting next departure** appears beside the prayer while the gift is unused; the god's waiting time is shown separately. Other ongoing gifts show their remaining game days.

In Keryx, `rite --list` shows your prayers, their effects, offerings, active game days, pending departure gifts and each temple's rest. `rite --prayer <id>` chooses one. Omitting the ID uses the prayer marked as the culture's default — Mistress of the Walls for Minoans.

## The value of an offering {#offerings}

[[bronze|Bronze]] can weigh far more with the gods than grain. The gods keep a current reckoning: how much each good counts per unit, before a particular god's preferences. What the world can spare and what the god favours both change an offering's meaning, so the figures move and are not listed here. `keryx cult` and `keryx rite --list` print the current values (bronze can weigh many times what grain does); the web prayer row shows only what each god **Favours**.

These are offering values. Wanaxes agree their own trade prices. You may compose an offering in the Offer row on the web, or in Keryx with `rite --offer good=amount,...`. Each god has a taste: the Offer row's **Favours:** line lists the goods this god loves most, dearest first. The traditional recipe always suffices.

Any good may be offered. **Wine and oil** are offerings like any other — oil is also food, so do not give up what a hungry city needs. **Incense** is a valuable gift ([[incense]]). For everyday prayers it can help meet the offering requirement; excess value does not change the answer, but a lasting prayer lasts longer and the temple rests longer.
