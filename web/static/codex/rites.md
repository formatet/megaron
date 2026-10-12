A **rite** is a request to the gods. Open **Kult → Prayers**.

Each prayer names its god, the minimum standing it needs, and its traditional offering. Press **Offer**. The temple brings that prayer's recipe.

## A rite is a wager

The offering is consumed whether the gods answer or not, and the rite **spends kharis** either way. Its chance is essentially your standing ([[kharis]]). You are never shown the exact odds — only the gods' mood, and whether they seem worth asking. That is on purpose.

For the Minoan prayers, a successful answer starts a waiting time for that god across all your cities. A silent answer takes the offering and kharis, but starts no waiting time. Potnia's two prayers share her 24-day wait. An active gift cannot be renewed before it ends.

## The prayers

- **The oracle** — names one ore deposit you have never seen — copper, tin or silver, anywhere in the world, chosen by the gods — and lifts the fog from it and the land around it. Whether you can *reach* it is another matter ([[bronze]], [[sea]]).
- **The harvest blessing** — for 25 days your grain, livestock, oil and wine yield a quarter more. Fish is not the fields' to give.
- **Battle frenzy** — strengthens a garrison for a while ([[battle]]).

The answer or a refusal appears in the Kult drawer after you offer; it stays beside the prayers.

An ongoing **blessed harvest** appears beside Food in the city drawer and in
`keryx status` and `keryx city`, with its yield bonus, affected goods and days left.
The row disappears when the blessing ends; fish receives no harvest bonus.

## The Minoan prayers

- **Potnia's Vision** — offer 2 grain + 4 silver. Potnia marks one ore deposit you neither see nor remember, and unknown ground within two hexes, as grey memory. It creates no living eyes and shows no moving activity. Potnia waits 24 days after answering.
- **Mistress of the Walls** — offer 3 grain + 1 timber. Potnia strengthens your own garrison defending this city by a quarter for 6 days. It helps neither an attacking army nor soldiers away from that city. It shares Vision's wait.
- **Green Shoots** — offer 2 grain + 2 silver. Velchanos raises grain, livestock, oil and wine yield by a quarter for 25 days. Fish is unchanged. He waits 25 days after answering.

Vision and Walls require at least 5 kharis; Green Shoots requires 30. An ongoing gift cannot be renewed; its remaining days appear beside the prayer. The temple's ordinary daily offering is separate.

In Keryx, `rite --list` shows your prayers, their effects, offerings, active days and waiting time. `rite --prayer <id>` chooses one. Omitting the ID uses the prayer marked as the culture's default — Mistress of the Walls for Minoans.

## Silver at the altar

Silver has a minimum value in a composed sacrifice, so it can be a meaningful gift. This changes how the gods reckon an offering; it changes neither market prices nor your silver production. You may compose an offering in Keryx with `rite --offer good=amount,...`; the web's Offer button brings the traditional recipe.
