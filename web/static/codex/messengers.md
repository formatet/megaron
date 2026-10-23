A **messenger** is a person who walks your letter to another Wanax's city — and walks the reply back. The **Diplomacy (✉)** drawer holds everything: tabs **Queue**, **Correspondence**, **Known** and **Runners**.

## Incoming queue

**Diplomacy → Queue** opens first. **Waiting on you** shows received letters awaiting a reply and open trade offers, even when you cannot afford the requested goods or silver. **Past correspondence** retains received letters and resolved offers. **Expired without answer** means the deadline passed; it does not mean the other Wanax actively declined.

The queue shows each letter separately. Groups apply to the displayed page, with newest letters first inside each group. **Older correspondence →** opens the next page; there may be older unanswered letters there. **Refresh from newest** starts again with current states. Letters sent after you began paging appear on refresh. Existing conversations and outgoing letters remain under **Correspondence**.

A row shows its offer, payment blocker and local answer deadline before you open it. **Read letter** marks only that opened letter as read. Reading never accepts, declines or replies. The server supplies available responses; an unaffordable open offer can still be declined. Accepting rechecks payment and transport, so the server may explain why a listed offer cannot currently be dispatched.

In Keryx, use `inbox --queue`, optionally `--limit 30` and `--cursor <next_cursor>`. `--json` returns the same page and opaque next cursor. Queue listing does not mark letters read. The ordinary `inbox` command still displays and marks its letters read; `inbox --history` keeps its existing history view.

## Writing

**Diplomacy → Known**: find a city or ruler and press **Write** to open a conversation. Write your letter and press **Dispatch →**. Rumours do not open a writing route. You can also write from a known city’s Inspect panel. Inside an open conversation, an incoming letter has a **Reply** box directly under it.

**Correspondence** groups everything into conversations, one per counterpart. A **⚖** badge marks a pending [[trade|trade offer]]; a **●** marks something unread.

## Letters take time

A letter to a distant city takes days, and the reply takes as long again. You are told when your messenger arrives, and again when he is home with the answer. Plan diplomacy the way you plan a march.

A reply uses the **same bearer** who brought the letter. You can return a letter even if you have not discovered the sender's city; answering does not create a new messenger or reveal that city's location.

A newly delivered plain letter can be answered on its arrival day and the following day. The bearer leaves at the start of the next day. **Queue** shows **Return post before** with the local departure time. Once the return-post window closes, you cannot reply through that bearer, even while his departure is still being processed. Reading does not extend his stay. A previously issued longer stay keeps its saved deadline. If an older letter has no saved deadline, the client says the deadline is unavailable and the server still decides whether its bearer can answer. Trade offers keep their own answer deadline.

If nobody replies, your messenger does not wait forever in that city's court. After a while he gives up and turns around on his own, walking home just as he would with an answer in hand — the same road, taking the same time. You are still told the moment he is back, only without a reply riding along.

## Messengers are sacred

**A messenger cannot be intercepted** — not on the way out, not on the way home. A herald is sacred and everyone knows it. It is the one thing on the map that is immune, which is why letters are the reliable backbone of everything you do at a distance.

A [[runners|Runner]] carrying an order is sacred too. Armies and goods are not. At sea, holiness protects a bearer from people, not a storm: see [[runner-lost-at-sea]] and [[runner-rescued-at-sea]].

The first letter you send to a city also opens [[contact]] with it for good.

## Crossing the sea

A messenger cannot swim any more than a Runner can. When a letter's road crosses open water it waits in your own port for a ship instead — see [[sea]] for how the crossing actually works.

If an order Runner is waiting for a ship, open its dispatch to see the sealed order's unit, destination and mission. Your Notifications history keeps those details readable after the Runner moves. Only your own order is shown; another Wanax's Runner does not reveal its envelope.
