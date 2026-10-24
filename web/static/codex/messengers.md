A **messenger** walks your letter to another Wanax and brings the answer home. **Diplomacy (✉)** has two tabs: **Letters** and **Runners**.

## Your letters

**Letters** has one row per Wanax: their name, the last line of the latest letter, and **Waiting on you**, **Waiting on them** or **Done**. Letters awaiting your answer come first, with the nearest deadline first. An unanswered offer remains actionable even when you cannot afford its payment.

Open a Wanax to read your exchange, oldest first. The newest letter is open; expand an older letter to read it. Opening a letter marks it read. Reading never accepts a trade or sends an answer. Offers have **Accept** and **Decline** buttons. An accepted offer remains in the exchange as **Agreed**, with its exact terms; agreement does not prove the goods have arrived.

The reply box sits below the exchange. **Send with their runner** uses the bearer waiting for your answer and shows when he leaves. **Send a runner** sends a new letter to a city you have contacted. **Write** lists the Wanax you can reach, with their contacted cities beneath. A rumour does not open a route. You can also write from a known city's Inspect panel. Unsent text stays in the box in this browser while you use the drawer; it is not saved to the server.

Keryx: `letters` lists your correspondents; `letters "Myrine"` reads your exchange with that Wanax. `--json` returns the same data. `reply --id <letter-id> --text "..."` answers with the waiting bearer. `message --to <city> --text "..."` sends a new runner; `trade-offer` attaches structured trade terms. `inbox` remains available for received letters and marks those displayed as read.

## Letters take time

A letter to a distant city takes days, and the reply takes as long again. You are told when your messenger arrives, and again when he is home with the answer. Plan diplomacy the way you plan a march.

A reply uses the **same bearer** who brought the letter. You can return a letter even if you have not discovered the sender's city; answering does not create a new messenger or reveal that city's location.

A newly delivered plain letter can be answered on its arrival day and the following day. The bearer leaves at the start of the next day. **Letters** shows the departure time with the local departure time. Once the return-post window closes, you cannot reply through that bearer, even while his departure is still being processed. Reading does not extend his stay. A previously issued longer stay keeps its saved deadline. Trade offers keep their own answer deadline.

If nobody replies, your messenger does not wait forever in that city's court. After a while he gives up and turns around on his own, walking home just as he would with an answer in hand — the same road, taking the same time. You are still told the moment he is back, only without a reply riding along.

## Messengers are sacred

**A messenger cannot be intercepted** — not on the way out, not on the way home. A herald is sacred and everyone knows it. It is the one thing on the map that is immune, which is why letters are the reliable backbone of everything you do at a distance. See [[xenia]] for the gods' order behind it.

A [[runners|Runner]] carrying an order is sacred too. Armies and goods are not. At sea, holiness protects a bearer from people, not a storm: see [[runner-lost-at-sea]] and [[runner-rescued-at-sea]].

The first letter you send to a city also opens [[contact]] with it for good.

## Crossing the sea

A messenger cannot swim any more than a Runner can. When a letter's road crosses open water it waits in your own port for a ship instead — see [[sea]] for how the crossing actually works.

If an order Runner is waiting for a ship, open its dispatch to see the sealed order's unit, destination and mission. Your Notifications history keeps those details readable after the Runner moves. Only your own order is shown; another Wanax's Runner does not reveal its envelope.
