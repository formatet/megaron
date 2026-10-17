A **rite** is a request to the gods. Open **Kult → Prayers**.

Each prayer names its god, the minimum standing it needs, and its traditional offering. Press **Offer**. The temple brings that prayer's recipe.

## A rite is a wager

The offering is consumed whether the gods answer or not, and the rite **spends kharis** either way. Its chance is essentially your standing ([[kharis]]). You are never shown the exact odds — only the gods' mood, and whether they seem worth asking. That is on purpose.

For the Minoan prayers, a successful answer starts a waiting time for that god across all your cities. A silent answer takes the offering and kharis, but starts no waiting time. Potnia's two prayers share her 24-day wait. An active gift cannot be renewed before it ends. A gift awaiting departure cannot be renewed before it is used.

## The prayers

The current Minoan prayers are **Potnia's Vision**, **Mistress of the Walls**, **Green Shoots**, **The Thread Shows a Path** and **Favourable Passage**, described below. `keryx rite --list` or **Kult → Prayers** shows the prayers your culture can offer and why one is unavailable. Current kharis is shown with a decimal; eligibility uses the exact pool value.

The answer or a refusal appears in the Kult drawer after you offer; it stays beside the prayers.

An ongoing **blessed harvest** appears beside Food in the city drawer and in
`keryx status` and `keryx city`, with its yield bonus, affected goods and days left.
The row disappears when the blessing ends; fish receives no harvest bonus.

## The Minoan prayers

- **Potnia's Vision** — offer 10 grain + 3 silver. Potnia marks one ore deposit you neither see nor remember, and unknown ground within two hexes, as grey memory. It creates no living eyes and shows no moving activity. Potnia waits 24 days after answering.
- **Mistress of the Walls** — offer 8 grain + 4 timber. Potnia strengthens your own garrison defending this city by a quarter for 6 days. It helps neither an attacking army nor soldiers away from that city. It shares Vision's wait.
- **Green Shoots** — offer 15 grain + 2 silver. Velchanos raises grain, livestock, oil and wine yield by a quarter for 25 days. Fish is unchanged. He waits 25 days after answering.

- **The Thread Shows a Path** — offer 4 timber + 3 silver to Asterion. The next land caravan or Runner leaving this city takes a quarter less time on its outward leg. A land caravan can be intercepted only within one hex of an enemy army, instead of two. Runners remain sacred. Asterion waits 12 days after answering.
- **Favourable Passage** — offer 6 fish + 3 silver to The Sea Daimons of Stillness. The next ship leaving this city takes a quarter less time and suffers no storm damage on its outward leg. A Runner aboard travels at that ship's actual speed. The Sea Daimons wait 12 days after answering.

Vision, Walls and the two departure prayers require at least 5 kharis; Green Shoots requires 30. The temple's ordinary daily offering is separate.

A departure gift stays in the city where you offered until an eligible journey physically leaves. It is used once. It cannot follow the city to a new owner, change a journey already underway, or bless a return leg. **Awaiting next departure** appears beside the prayer while the gift is unused; the god's waiting time is shown separately. Other ongoing gifts show their remaining days.

In Keryx, `rite --list` shows your prayers, their effects, offerings, active days, pending departure gifts and waiting time. `rite --prayer <id>` chooses one. Omitting the ID uses the prayer marked as the culture's default — Mistress of the Walls for Minoans.

## Work at the altar

The gods reckon offerings in **workdays**: the labour to produce each good on standard ground, including its ingredients. Grain and silver each represent one workday; bronze represents eleven, including nine copper, one tin and the smelter's work. World scarcity and the god's tastes lift this reference value. Silver receives no extra minimum value.

These are offering references. Trade prices are agreed between Wanaxes. You may compose an offering in Keryx with `rite --offer good=amount,...`; the web's Offer button brings the traditional recipe.
