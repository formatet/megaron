**Command is never instant.** This is the rule that makes Megaron different from most strategy games.

When you tell a unit in the field to turn around, change course or change stance, you do not reach the unit — you send a **Runner**, a real person who must walk to it. The game gives the next meeting estimate, such as *"Runner following the unit — next meeting today 22:40."* Until then the unit keeps doing what it was last told.

## How orders travel {#runners-details}

While the Runner is still on the road, the order shows under the unit in **War → Forces** and in `keryx army`, with the next meeting time — it disappears once he has delivered it. If the unit moves again, the meeting estimate can change; it does not promise that the order is already delivered.

The one exception is your [[horde|Nomadic Host]]: you travel with it in person, so it obeys at once.

## Order travel and warning time {#runners-why-this-is-good-for-you}

Because orders move at walking pace, **a defender who is offline is not helpless.** An attacker has to cross ground you can see, and your answer usually has less ground to cross than his army does. Distance is the defender's ally. See [[defence]].

## Failed orders {#runners-when-an-order-fails}

Sometimes a Runner arrives and the order can no longer be carried out. You get an *Order failed* notification that names the order and the reason. Look at the unit in **War → Forces** and decide again.

Recall and redirect are sent from **War → Forces**, or — for redirect — by right-clicking a new destination for a unit that is already marching. See [[marching]].

## Catching a moving force {#runners-catching-up-with-a-moving-unit}

A unit on the march does not wait for its orders. A Runner sent to it — to recall it, redirect it or change its [[stances|stance]] — is aimed at the point on its road where he can **catch up** with it, and the game tells you when he will. Runners are quicker than soldiers, but not endlessly: a recall or redirect that cannot catch the unit before it arrives still sends a Runner to its destination. For a newly sent land order, a missed meeting is not the end of the journey: if the unit has moved again, the Runner keeps the order and follows it along a new physical route. March, recall, redirect and stance orders take effect only when the Runner and the unit actually meet. The pending order says when its next meeting is expected and when it is following a new route. If it must wait before it can continue, the pending order says so without promising a delivery time. A Runner cannot deliver to a destroyed unit or one you no longer own.

### Sacred letters and order Runners {#runners-catching-up-with-a-moving-unit-2}

Letters to other Wanaxes travel the same way, but they are sacred and cannot be stopped: see [[messengers]].

## Runners crossing water {#runners-crossing-the-sea}

A Runner whose road to your unit crosses open water cannot swim it — it waits in your own port for a ship — anyone's — to sail that way, the same as a letter does. See [[sea]] for how the crossing works, and what happens if the ship carrying it is lost.

## The Runners tab {#runners-the-runners-tab}

**Diplomacy → Runners** (or `keryx runners`) shows every runner of yours that is still out, in groups: **waiting for a ship**, **on the way**, **aboard a ship**, **waiting for an answer**, and **on the way home**. A waiting runner says where it stands, since when, and why — and offers **Arrange passage** (one of your own free ships) or **Call back** (only when it waits in your own port). A runner **waiting for an answer** has reached the other Wanax's city and stands there until they answer — the tab says since when, and when it heads home if nobody does. An order's runner names the order it carries. Times are the clock time something happens — "today 18:34" — never a game day count.

The tab also lists other Wanaxes' runners, but only where you can know they are there: standing in one of your cities or ports, or aboard one of your ships. You see whose runner it is and where — never what it carries, and never the road it came by. Runners are sacred; a ship's captain does not read the letters. See [[messengers]] and [[sea]].

## Runner speed {#runners-runner-speed}

On land a Runner needs half a foot army's [[terrain|terrain]] time ([[marching]]). At an
ordinary [[rivers|river]] crossing he uses the ship's river cost; at a [[river-ford|ford]] he wades.
At sea he has no separate speed: he travels with the real ship carrying him.

A runner follows a saved land route one province at a time, including the walk to a harbour and the walk after landing. It sees from each province when it reaches it, and remembers that ground even while you are away. Runners remain sacred: they cannot be intercepted.

## Arranging a ship for a Runner {#arrange-passage}

Waiting for a ship to happen to sail the right way is not your only choice. From your **outbox** (Diplomacy, or `keryx outbox`), a runner waiting for passage shows an **Arrange passage** button (or `keryx passage`) whenever one of your own ships qualifies — the choice of ship is always yours to make, but only real, eligible ships are ever offered: a [[galley]] or [[emporos]] (never a [[war-galley]]), standing free in port with no mission of its own.

- **Sending one out:** the ship must stand in the SAME port the runner is waiting in. It sails straight for the runner's destination, boards it the moment it leaves, and puts it ashore there — no different from boarding any ship that happened to be going that way, except this one you chose yourself.

### Fetching a waiting Runner home {#arrange-passage-2}

- **Bringing one home:** a runner that has finished its errand and is waiting in a **foreign** port may be fetched instead — any of your own ports will do. The ship sails there, waits (**War → Forces** shows it "waiting at (…) for the runner's return", or `keryx unit` says the same), and carries the runner all the way home the moment it is ready to travel.

- Either way, **the ship still finishes its own mission on its own** afterwards — it always sails home again, exactly like Land, Patrol or Explore. The runner never commands the ship; you send the ship for the runner, never the other way round. While it sails out, the drawer and `keryx unit` show it "carrying a runner to (…)".

- The ship's own risks are unchanged: it can still be damaged, captured or sunk, and a starved crew still turns it for home ([[upkeep]]) — none of that touches the runner, who stands safely ashore the whole time.

## A runner lost at sea {#runner-lost-at-sea}

A [[storms|storm]] can sink a runner's ship. If a ship sinks in [[battle|battle]] and no enemy ship survives to rescue the runner, the runner is lost too. This applies to [[messengers|letters]], [[trade|trade offers]] and [[runners|orders]].

If the crew disappears from [[upkeep|hunger or desertion]], the runner steps ashore if it can reach a nearby settlement. Out at sea, with no way to leave, it is lost.

You learn immediately that **your own runner was lost**. This is the one exception to news needing a physical bearer. The dispatch tells you nothing else about the battle or other people’s ships.

### The lost Runner’s sealed contents {#runner-lost-at-sea-details}

Open the dispatch to read the whole sealed contents: the letter and any reply, the trade proposal or the order, its recorded departure time (for example, "Sent yesterday 18:34"), and its origin and destination. The record does not change when cities or orders change later.

A lost runner cannot deliver the message later. Sending again is your decision. If another ship rescues it, see [[runner-rescued-at-sea]].

An older sealed record may have no preserved departure game day; it says "Sent on an unknown game day" rather than guessing.

The dispatch names people first: "From you, at Mycenae, to Wanax Oledoledoff at Tiryns." An order names "your [[elite-infantry]] at (9, 4)" instead. The recipient's public Wanax name, your unit's name and the places are preserved when the runner is lost; later name changes do not rewrite the sealed record. Account logins are not used.

### Reading a Runner loss report {#runner-lost-at-sea-details-2}

The headline names only the recipient: "Your runner to Wanax Oledoledoff at Tiryns was lost at sea in a storm." For an order: "Your runner to your Bronze Guard at (9, 4) was lost at sea in a storm." The full origin stays in the sealed envelope below.

## Passage aboard a ship {#runner-passage}

A [[messengers|letter]] or a [[runners|Runner]] carrying your order cannot swim, and it never guesses at a crossing — it needs a real ship, full stop. When its road crosses open water:

- It runs to **your own nearest coastal or harboured city** and waits there — you are told which one, and why. If you have no coastal or harboured city anywhere, the message is refused outright at the moment you send it, rather than dispatched to wait forever.

### Boarding any suitable ship {#runner-passage-2}

- The moment **any** ship or trade route — yours or another Wanax's — sails from that port toward the right side of the sea, it **boards** — free, no cargo space taken — and steps off wherever that ship lands, then finishes the trip on foot. Runners are sacred: every captain takes them aboard, even between Wanaxes at war. It only boards a ship that lands it **nearer its goal** than the port it waits in — a ship bound the wrong way, it lets sail. The runner never changes where the ship goes, and the ship's owner sees only that a runner of yours is aboard, never what it carries.

- A storm can [[runner-lost-at-sea|lose the runner]]. In battle a surviving enemy ship [[runner-rescued-at-sea|rescues it]]; a captured ship keeps it aboard. It leaves that actual ship in its next port and continues. A damaged ship that survives takes its runner home to the next port, without sealing or loss.

### Waiting for a sailing {#runner-passage-3}

- If no ship sails that way for a while, it simply keeps waiting — nothing crosses on its own. See "A runner stuck waiting" below for what you can do about it.

None of this changes anything for a message or order whose road stays on dry land — a Runner on land never needs a ship, only [[marching|the roads]] it can already walk (a river still needs a ford or a boat of its own, unrelated to sea passage).

## Passage markers on the map {#runner-passage-on-map}

Your own [[messengers|letter]] or [[runners|Runner]] is drawn exactly where it really is, not sailing on its own across open water:

- **Waiting** — it stands still in the port it ran to, a gold pennant marking it as yours. A small red mark on top means a **PassageStalled** dispatch has already fired for this wait.
- **Aboard** — it rides the deck of whichever ship or trade route picked it up, moving exactly when and where that ship does. Hover it to see the carrier's name.
- **Ashore** — once the ship lands it, it steps off and finishes the last stretch on foot, same as any Runner on land.
- **No word** — after a rescue its location is unknown to you: no marker, carrier name or sight. It tells you about the detour only when it physically comes home. Sealed markers remain a recovery measure for older unresolved journeys.

## A runner rescued at sea {#runner-rescued-at-sea}

A herald is sacred. When its ship sinks in battle, a surviving enemy ship takes the runner aboard. A captured ship also keeps its runner safe, even though its owner has changed. The new owner cannot read the sealed message or hold the runner captive.

The runner follows that **actual ship to its next port**, steps ashore and continues toward the original destination. It may need to wait for another passage. A damaged ship that survives and turns home keeps its runner aboard until that home port; that is neither a loss nor a rescue.

### Rescue and the next physical port {#runner-rescued-at-sea-details}

You receive no instant rescue news or location. Your outbox says **no word**, and the runner's unknown position gives you no map marker or sight. The recipient simply receives the message later.

The report says when the runner came home, such as "Home today 18:34". When the runner comes home with its answer, it brings its own account of the rescue ship and the port where it landed. A one-way order runner does not send a report home from afar. Only [[runner-lost-at-sea|loss]] is reported immediately. See [[sea]] and [[messengers]].

The account names the ship and the actual towns in a sentence: "Home today 18:34. Your runner was rescued at sea by the Sacred Dolphin and put ashore at Tiryns, then went ashore at Mycenae." With only one landing: "Home today 18:34. Your runner was rescued at sea by the Sacred Dolphin and put ashore at Tiryns." The town names are preserved when the runner steps ashore.

## Calling a Runner home {#runner-return}

In **Diplomacy → Correspondence**, use **Call it back** on the waiting runner's row, or use the same choice in its **PassageStalled** dispatch. In Keryx, find the runner with `outbox`, then use `call-back --id <messenger-id>`.

This works only for an outbound runner waiting for a ship in **your own port**. A runner already aboard a ship, or waiting in a foreign port on its return journey, cannot be called back this way — arrange passage instead.

The runner turns around and walks home over land, delivering nothing. The confirmation tells you when it will return; it is still travelling until then. A letter comes home undelivered, and an order comes home withdrawn.

## A Runner awaiting passage {#runner-waiting}

Wait long enough with no ship sailing the right way, and you get a **PassageStalled** dispatch: the runner, the port it's standing in, where it's trying to reach, and the ships (if any) of yours already there. Nothing forces your hand — it keeps waiting exactly where it is until you act, and the dispatch only ever fires once per spell of waiting (a later known waiting spell can trigger it again; an unknown rescued runner sends no news from a foreign port). Three choices:

- **Arrange passage** — the same button as above, right there in the dispatch.
- **Call it back** — give up on this errand and bring the runner home undelivered. See the instructions below.
- **Let it wait** — close the dispatch and do nothing. It keeps waiting, exactly as it was.
