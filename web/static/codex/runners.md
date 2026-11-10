**Command is never instant:** to change a unit in the field you send a **Runner**, who walks to it.

- On land a Runner needs half a foot army's terrain time.
- An order takes effect only when Runner and unit meet; until then the unit keeps its last orders.
- Runners are sacred and cannot be intercepted. Your [[horde|Nomadic Host]] obeys at once.
- A Runner cannot swim: it waits in your port for any ship sailing the right way.
- Your own lost runner is reported at once. <!-- src: server/internal/province/pathfind.go runner speed -->

## How orders travel {#runners-details}

The game gives the next meeting estimate, such as *"Runner following the unit — next meeting today 22:40."* While the Runner is on the road the order shows under the unit in **War → Forces** and `keryx army`, and disappears once delivered. If the unit moves again the estimate can change; it never promises delivery.

## Order travel and warning time {#runners-why-this-is-good-for-you}

Because orders move at walking pace, **a defender who is offline is not helpless**: an attacker crosses ground you can see, and your answer usually has less ground to cross. Distance is the defender's ally ([[defence]]).

## Failed orders {#runners-when-an-order-fails}

If a Runner arrives and the order can no longer be carried out, you get an *Order failed* notification naming the order and the reason. Recall and redirect are sent from **War → Forces**, or by right-clicking a new destination ([[marching]]).

## Catching a moving force {#runners-catching-up-with-a-moving-unit}

A Runner sent to a marching unit is aimed at the point on its road where he can **catch up**, and the game tells you when. Runners are quicker than soldiers, but if a recall or redirect cannot catch the unit before it arrives, the Runner goes to its destination. For a newly sent land order, if the unit has moved again, the Runner keeps the order and follows by a new route. March, recall, redirect and [[stances|stance]] orders take effect only when they meet. A Runner cannot deliver to a destroyed unit or one you no longer own.

### Sacred letters and order Runners {#runners-catching-up-with-a-moving-unit-2}

Letters to other Wanaxes travel the same way, sacred and unstoppable: [[messengers]].

## Runners crossing water {#runners-crossing-the-sea}

A Runner whose road crosses open water waits in your own port for a ship, anyone's, to sail that way, as a letter does. See [[sea]] for the crossing and for a lost ship.

## The Runners tab {#runners-the-runners-tab}

**Diplomacy → Runners** (or `keryx runners`) groups your runners still out: **waiting for a ship**, **on the way**, **aboard a ship**, **waiting for an answer**, **on the way home**. A waiting runner shows where, since when and why, and offers **Arrange passage** (one of your free ships) or **Call back** (only in your own port). A runner awaiting an answer stands in the other Wanax's city until they reply; the tab says when it heads home if nobody does.

Other Wanaxes' runners appear only where you can know they are: in one of your cities or ports, or aboard your ships. You see whose and where, never what it carries or its road ([[messengers]], [[sea]]).

## Runner speed {#runners-runner-speed}

On land a Runner needs half a foot army's [[terrain|terrain]] time ([[marching]]). At an
ordinary [[rivers|river]] crossing he uses the ship's river cost; at a [[river-ford|ford]] he wades.
At sea he travels with the real ship carrying him. He follows a saved land route one province at a time, sees from each province he reaches and remembers that ground.

## Arranging a ship for a Runner {#arrange-passage}

From your **outbox** (Diplomacy, or `keryx outbox`), a runner waiting for passage shows **Arrange passage** (or `keryx passage`) whenever one of your ships qualifies: a [[galley]] or [[emporos]] (never a [[war-galley]]), free in port with no mission.

- **Sending one out:** the ship must stand in the SAME port as the runner. It sails straight for the runner's destination, boards it as it leaves and puts it ashore there.

### Fetching a waiting Runner home {#arrange-passage-2}

- **Bringing one home:** a runner waiting in a **foreign** port after its errand can be fetched to any of your ports. The ship sails there, waits (**War → Forces** says "waiting at (…) for the runner's return") and carries the runner home when it is ready.
- Either way **the ship finishes its own mission** afterwards and sails home. The runner never commands the ship. The ship's own risks are unchanged and do not touch the runner ashore.

## A runner lost at sea {#runner-lost-at-sea}

A [[storms|storm]] can sink a runner's ship. If a ship sinks in [[battle|battle]] and no enemy ship survives to rescue the runner, it is lost too. This covers [[messengers|letters]], [[trade|trade offers]] and [[runners|orders]]. If the crew vanishes from [[upkeep|hunger or desertion]], the runner steps ashore if a settlement is near; otherwise it is lost.

You learn immediately that **your own runner was lost**: the one exception to news needing a physical bearer. The dispatch says nothing else about the battle.

### The lost Runner’s sealed contents {#runner-lost-at-sea-details}

Open the dispatch to read the sealed contents: the letter and any reply, the proposal or the order, its departure time and its origin and destination. A lost runner cannot deliver later; sending again is your decision. If another ship rescues it, see [[runner-rescued-at-sea]].

### Reading a Runner loss report {#runner-lost-at-sea-details-2}

The headline names only the recipient: "Your runner to Wanax Oledoledoff at Tiryns was lost at sea in a storm." The full origin stays in the sealed envelope. Names are preserved as they were when the runner was lost.

## Passage aboard a ship {#runner-passage}

A [[messengers|letter]] or a [[runners|Runner]] carrying your order needs a real ship. When its road crosses open water it runs to **your own nearest coastal or harboured city** and waits there; you are told which one. With no such city the message is refused when you send it.

### Boarding any suitable ship {#runner-passage-2}

- When **any** ship or trade route, yours or another Wanax's, sails from that port toward the right side of the sea, the runner **boards** free of charge and steps off where it lands, then walks on. Every captain takes runners aboard, even between Wanaxes at war. It boards only a ship that lands it **nearer its goal**. The ship's owner sees only that a runner of yours is aboard.
- A storm can [[runner-lost-at-sea|lose the runner]]; in battle a surviving enemy ship [[runner-rescued-at-sea|rescues it]]; a captured ship keeps it aboard. A damaged ship that survives takes it to the next port.

### Waiting for a sailing {#runner-passage-3}

If no ship sails that way, the runner keeps waiting ([[runner-waiting]]). A runner on land never needs a ship, only [[marching|the roads]] (a river still needs a ford or boat).

## Passage markers on the map {#runner-passage-on-map}

Your own [[messengers|letter]] or [[runners|Runner]] is drawn where it really is:

- **Waiting:** standing in its port, a gold pennant marking it yours; a red mark means a **PassageStalled** dispatch has fired.
- **Aboard:** on the deck of the ship that took it; hover for the carrier's name.
- **Ashore:** once landed, it walks the last stretch.
- **No word:** after a rescue its location is unknown: no marker, carrier name or sight.

## A runner rescued at sea {#runner-rescued-at-sea}

A herald is sacred. When its ship sinks in battle, a surviving enemy ship takes the runner aboard; a captured ship keeps it too. The new owner cannot read the message or hold the runner. The runner follows that **actual ship to its next port**, steps ashore and continues, perhaps waiting for another passage. A damaged ship that survives keeps its runner to its home port: neither loss nor rescue.

### Rescue and the next physical port {#runner-rescued-at-sea-details}

You get no instant rescue news, location or marker; your outbox says **no word**. When the runner comes home with its answer it brings its account: "Home today 18:34. Your runner was rescued at sea by the Sacred Dolphin and put ashore at Tiryns." A one-way order runner sends no report from afar. Only [[runner-lost-at-sea|loss]] is reported immediately. See [[sea]] and [[messengers]].

## Calling a Runner home {#runner-return}

**Call it back** on the waiting runner's row in **Diplomacy → Correspondence** or in its **PassageStalled** dispatch; in Keryx, `outbox` then `call-back --id <messenger-id>`. This works only for an outbound runner waiting in **your own port**; for one aboard a ship or waiting in a foreign port, arrange passage instead. The runner walks home delivering nothing: a letter comes home undelivered, an order withdrawn.

## A Runner awaiting passage {#runner-waiting}

After a long wait with no ship sailing the right way you get one **PassageStalled** dispatch naming the runner, its port, its goal and your ships there. Nothing forces your hand. Choose **Arrange passage**, **Call it back**, or **let it wait**.
