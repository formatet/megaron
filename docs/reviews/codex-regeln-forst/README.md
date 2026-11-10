# Codex: the rule first (2026-11-10)

## Word counts

| article | before | after | % | lead words |
|---|---|---|---|---|
| rites | 2934 | 1660 | 56 | 78 |
| runners | 2482 | 1479 | 59 | 90 |
| kharis | 2078 | 1233 | 59 | 83 |
| ship-types | 1792 | 1067 | 59 | 76 |
| marching | 1719 | 1020 | 59 | 90 |
| temples | 1638 | 980 | 59 | 78 |
| buildings | 1426 | 838 | 58 | 81 |
| units | 1413 | 797 | 56 | 79 |
| total | 15482 | 9074 | 58 | |

## The eight leads

### rites

```
A rite is a request made at the temple of its god.

- Each prayer is said at its own god's [[temples|temple]]; level 1 is enough.
- [[kharis]] is a minimum you must hold, never spent; the offering is.
- Bring the traditional recipe, or a composed offering worth at least as much.
- An answered prayer makes **that temple** rest, and only while it is fully staffed.
- A larger gift makes a lasting prayer last longer. <!-- src: reference.json prayers -->
```

### runners

```
**Command is never instant:** to change a unit in the field you send a **Runner**, who walks to it.

- On land a Runner needs half a foot army's terrain time.
- An order takes effect only when Runner and unit meet; until then the unit keeps its last orders.
- Runners are sacred and cannot be intercepted. Your [[horde|Nomadic Host]] obeys at once.
- A Runner cannot swim: it waits in your port for any ship sailing the right way.
- Your own lost runner is reported at once. <!-- src: server/internal/province/pathfind.go runner speed -->
```

### kharis

```
**Kharis** is your standing with [[gods|the gods]], held for your whole realm.

- A new Wanax starts at 40; it never falls below 1.
- Each active city adds to the realm's demand, which fades kharis every game day.
- Staffed, fed temples raise it; your best temple sets the ceiling (50, 75, 100; 40 with none).
- Below 30 the gods may punish you; a punishment gives 7 game days of protection.
- Prayers need a minimum standing but never spend it. <!-- src: reference.json kharis -->
```

### ship-types

```
Every ship needs a **[[harbour|Harbour]]** and a **[[shipyard|Shipyard]]** ([[buildings]]).

- A Galley ([[timber|timber]]) carries 20 goods or one land unit.
- An Emporos (90 [[cedar|cedar]]) carries 120 goods or up to three land units.
- A War Galley (cedar, [[foundry]]) fights and never carries cargo.
- A ship takes its whole mission in port; once it sails it takes no more orders.
- It eats what it carries and turns home before the food runs short. <!-- src: server/internal/unit catalogue; reference.json units -->
```

### marching

```
**Right-click a destination**, select your forces and check the forecast before sending.

- A plain march reaches only ground your people have seen; **Explore** covers unknown ground.
- Foot soldiers take 1 game day per plain hex, up to 3 in semi-desert; mountains cannot be crossed.
- Orders to forces in the field travel by Runner and take effect when it reaches them.
- Ships get their whole mission in port ([[ship-types]]).
- An enemy can stop you and a [[battle|battle]] begins by itself: there is no declaration of war. <!-- src: server/internal/province/movement.go TerrainMoveTicks -->
```

### temples

```
A **[[temple]]** ([[buildings]]) is how you keep [[gods|the gods]]' favour ([[kharis]]).

- A city may hold several temples, **at most one per god**: [[asterion]], [[potnia]], the [[sea-daimons|Sea Daimons]] of Stillness, [[velchanos]].
- Levels 1/2/3 have 2/4/8 workplaces; an unstaffed temple gives nothing.
- Each temple pays a daily tithe from its own city; unpaid parts lose effect.
- Your best temple sets your kharis ceiling: 50, 75 or 100.
- Demolishing a temple costs 10 kharis per level. <!-- src: reference.json kharis levels, demolition_per_level -->
```

### buildings

```
Buildings provide workplaces, defences and the facilities needed for recruitment and trade.

- One building is built at a time; two more wait in the queue.
- Farm, Lumbermill, Mine and Stone Quarry stand on one catchment hex, one per hex, with 4 workplaces each.
- Levels go to 3; each raises every worker's rate on that hex by 70 %.
- The wall sets the population ceiling: 2 000, then 5 000, 10 000, 20 000. <!-- src: server/internal/economy/population_cap.go populationCaps; hex_rules.go -->
- Demolishing refunds nothing.
```

### units

```
Forces are the soldiers, ships and people you command.

- Land forces train as a 100-man cohort drafted from one city's people; ships are built one at a time.
- Each force needs its building: Barracks, Stable or Shipyard (Foundry for Bronze Guard and War Galleys).
- Every force eats each game day; Bronze Guard, chariots and War Galleys also draw silver.
- Home service costs half the field pay.
- Open the **War (⚔)** drawer: **Forces**, **Recruit**, **Marches**. <!-- src: server/internal/unit catalogue; reference.json units -->
```


## Removed text (first six words · reason)
Removed or collapsed text, per article: first six words · reason.

## units (1413 -> see table)
- "Choose the city at the top" · screen description (kept: unavailable row names what is missing)
- "Cedar specifically required by an order" · cut as over-detail of cedar/timber rule (the one-for-one rule kept)
- "The completion dispatch names the ready" · screen description
- "Rowers eat like spearmen" (rations detail) · duplicate of [[upkeep]] (kept in short form)
- "In each combat round, 100 active" · duplicate of [[battle]] (kept War Galley 5 dice vs Galley 1 only as battle detail, then cut: also on battle/ship pages) 
- "There is no soldier-quantity input" and the receipt example · screen description / example
- "The card says Laid up and offers" · screen description
- "Every unit you own, with its state" · screen description
- "Forces on a hex: select their current" · screen description; foreign-force estimate rule kept in one line
- "Recruitment confirms that your order" · screen description
- "Forces groups ... Being in harbour" and "Marches lists your ordered marches ..." · screen description
- "Older units, army and unit list commands" · history

## buildings
- "Open City → Buildings. The tab" · screen description (moved rule bullets to the lead)
- "Construct shows the next wall greyed" · screen description
- "The founding forecast tells you" · duplicate of [[founding]]
- "You may build more than one" / "Building the same kind again" · example / duplicate of upgrade section
- "Harbour levels 1, 2 and 3 open" · duplicate of [[harbour]] / [[fishing]]
- "A building costs what it is made of: timber for ..." · screen description (prices are in Construct)
- "The market has no workplaces and" · history (former market workers)
- "If cancelling a queued build is refused" · screen description
- "for a lumbermill use keryx place" · example
- "In the Buildings tab press Demolish" · screen description
- "The confirmation shows the actual loss" · screen description
- "`keryx build --list [city]` ... Construction dispatches name" · screen description
- The generated building table (`<!-- list: building -->`, about 250 words) is untouched and counts toward the cap.

## temples
- "They are named by their god" · history/obvious
- "Each prayer is said at its own" list (swift-feet at Asterion's ...) · duplicate of [[rites]]
- "Demolishing a temple frees only" (build section) · duplicate of [[temple-demolition]]
- "City → catchment & workplaces → Temple: place" · screen description
- "The web shows one card per temple" · screen description
- "The tithe of each level is in the table" · duplicate of the table
- "Dearer goods, and goods the god prizes" · duplicate of the tastes table (kept as one clause)
- "Kult → Temples and keryx status show the next tithe" · screen description (one line kept)
- "With full staffing and tithe, a level-1" · CONTRADICTION removed (see below)
- "Everyday prayers work at a level-1 temple" · duplicate of [[rites]]
- "Demolishing a temple voluntarily costs 10 kharis" (temple-levels-2 body) · duplicate of [[temple-demolition]]; the heading and anchor stay
- "Manage in City opens that temple's city ... Temples you have not built are listed last" · screen description
- "Kult → Prayers lists prayers across" detail · screen description
- "One, two, three and four cities demand 1, 3, 6" · duplicate of the table in [[kharis]]
- "Kult → Kharis explains your realm's standing" · screen description
- "Staffing and paid tithe both multiply" · duplicate of weight rule above
- "A game day follows this world's pace" · screen/explanation duplicate of time article
- "The confirmation shows... Keryx asks for confirmation" detail, "The result and the god is angered notification" · screen description
- "Another equally high temple can keep" detail trimmed; "Once standing is at or below the ceiling" · repeats the line before

## marching
- "Only one unit is selected initially" · screen description
- "You can also left-click a hex and" · screen description (Hovering those buttons previews...)
- "Choose the named forces to send and" · duplicate of the sentence above it
- "Ships with different crews or cargo" · obvious/duplicate of forecast
- "Redirects also wait for a messenger" · duplicate of the Redirect section
- "Add --json for the server forecast" · screen/CLI detail
- "Selecting one fills the destination" · screen description
- "A notification tells you when" · screen description
- "Right-click a new destination and a unit" (menu detail) · screen description
- "The horn sounds when the soldiers" · colour/history
- "Recall and redirect ... While the Runner follows a moving unit" detail · duplicate of [[runners]]
- "The map and keryx unit list show" · screen description
- "Choose Explore ... default duration is ready" · screen description
- "The order summary shows its extent" and "show the area, duration, latest turn" · screen description
- "Choose a reachable hex or use a unit" · screen description
- "An expedition preview shows its planned length" / "A march preview applies" detail · duplicate of [[exploration]]
- "Before sending an expedition, both the web menu" · screen description
- "Travel time is fixed at departure; later rule changes" · history (kept: fixed at departure)
- "Your dispatch receipt says when" · screen description

## ship-types
- "The Galley is cheap timber and no silver" · duplicate of the unit pages and [[units]]
- "With automatic routing, no free hull" detail · shortened
- "From that unit's own card (Fetch by ship, whenever" · screen description
- "exactly like arrange passage's own crossing" · duplicate of [[messengers]]
- "You choose how many game days it holds" / "you are told either way" · screen description
- "Give a pickup mission while the ship is in port" · duplicate of the lead bullet
- "In War → Forces, use Load for each cohort" · screen description
- "You do not need a second Unload order. An empty ship" · duplicate/obvious
- "A laden ship can also use --intent land" · CLI detail
- "Deep water is faster, and storms" / "Rivers allow ... across or where the flow is unknown" · duplicate of [[marching]]
- "For goods, a Galley carries 20 and an Emporos" · duplicate of [[units]] (kept there)
- "Nike came home early" example, "It does not mean that the crew starved" · example
- "At sea, a ship eats" / hospitality host-notice sentence · duplicate of [[xenia]]
- "you are notified when the work is done" · screen description
- "Choose the mission in War → Forces" · screen description
- "Land" and "March to your city" table rows · duplicate of Docking / Landing sections
- "For Explore, the port loads food" · duplicate of [[sight]]
- "Load, March, Recall, Redirect ..." detail · shortened
- "Check supplies: War → Forces or keryx unit list" and the "At port" bullet · screen description / duplicate of [[xenia]]
- "Lay up an idle ... The crew joins" detail · duplicate of [[units]]
- The generated unit list (`<!-- list: unit ... -->`) is moved under a new un-anchored heading "The three ships" so the lead stays short; untouched otherwise.

## kharis
- "Kharis is your standing ... The Kult (⛩) drawer shows" · screen description
- "The number is your standing, not a percentage" · obvious; "Protection temporarily removes" · duplicate of Divine wrath
- "Blessings and punishments arrive as notifications" · screen description
- "Rising again keeps your standing... Kharis never falls all the way" · duplicate of the generated standing table (floor)
- "a fully staffed level-1 temple raises kharis only a quarter" · CONTRADICTION removed (see below)
- "1. You have a temple ... 2. People serve ... 3. The tithe is paid" list · duplicate of [[temples]]
- "In a small realm, a fully staffed" · example
- "it is more when the cult's reference offering of grain is scarce in the world" · FACTUAL ERROR (order item 4), removed
- "Larger temples can employ more devotion" · duplicate of [[temples]]
- "Your best temple sets your ceiling ... Everyday prayers work" (raising-kharis-2) · duplicate of the generated ceiling table and [[temples]]
- "Kharis changes by ... the second adds 2, the third 3" · duplicate of [[cult-upkeep]] and the generated demand table
- "Under Kharis each game day the drawer lists" · screen description
- "Kharis is a requirement for prayers, never spent" (second statement) · duplicate of the lead
- "An ongoing blessed harvest appears beside Food" · screen description
- "The temple shows how much each good counts" · duplicate of [[temples]] tastes
- "Demolishing a temple voluntarily costs 10 kharis" (full paragraph) · duplicate of [[temple-demolition]]
- "The Kult drawer has three tabs: Kharis for this realm-wide" detail · screen description
- "The estimate assumes that net continues" · obvious
- "A game day follows this world's pace; Kult ... show the protection's end" · duplicate of the time article
- "Repeated processing does not extend it. At the end" · internals
- "Everyday prayers work at a level-1 temple. Spontaneous blessings need 60" (divine-wrath-temple-levels) · third copy of the same text (heading and anchor stay)
- The three generated blocks (about 620 words) are untouched and count toward the cap.

## runners
- "When you tell a unit in the field to turn around" · duplicate of the lead (kept the meeting-estimate example, one example)
- "The one exception is your Nomadic Host" · moved to the lead
- "Look at the unit in War → Forces and decide again" · screen description
- "Recall and redirect are sent from War → Forces" (long form) · duplicate of [[marching]]
- "The pending order says when its next meeting" and "If it must wait before it can continue" · duplicate of How orders travel
- "In groups: ... Times are the clock time" detail · screen description
- "Runners are sacred; a ship's captain does not read" · duplicate of [[messengers]]
- "At least ... It sees from each province" history-style detail · shortened
- "Waiting for a ship to happen to sail the right way" · screen description
- "except this one you chose yourself" · obvious
- "While it sails out, the drawer and keryx unit show it" · screen description
- "The ship's own risks are unchanged ... starved crew" · duplicate of [[upkeep]] / [[sea]]
- "Open the dispatch to read ... (for example, 'Sent yesterday" · example
- "An older sealed record may have no preserved departure" · history
- "The dispatch names people first: 'From you, at Mycenae'" · example (second example removed; the headline example kept)
- "A runner follows a saved land route" duplicate sentences · shortened
- "None of this changes anything for a message" · shortened
- "Sealed markers remain a recovery measure" · history
- "The runner follows that actual ship ... You receive no instant" second example "With only one landing" · example
- "In Diplomacy → Correspondence, use Call it back" long form · screen description
- "(a later known waiting spell can trigger it again; an unknown rescued runner" · internals

## rites
- "A **rite** is a request to the gods. Open Kult" · duplicate of the first line
- "Each prayer row shows its temple, Required kharis" · screen description
- "Each of the ten Minoan everyday prayers" · CONTRADICTION (fourteen prayers listed), removed (see below)
- "Choose some goods before bringing the gift" and "In an interactive terminal, Keryx shows" · screen description
- "For these prayers, --offer-multiplier is ignored" · CLI internals
- "Another temple of the same god in another city" · obvious from "that temple"
- "Kult and keryx rite --list show the rest" · screen description
- "Older prayers outside the current Minoan catalogue retain" · history
- "The current Minoan prayers are Following Seas, Fruitful" · duplicate of the list below and of [[kharis]] table
- "Current kharis is shown with a decimal" · screen description
- "The answer or a refusal appears in the Kult drawer" · screen description
- "An ongoing blessed harvest appears beside Food" · duplicate of [[kharis]] (screen description)
- Per-prayer "offer 15 grain + 2 silver ... His temple rests 25 game days" (14 lines) · duplicate of the generated table in [[kharis]] and of each prayer's own article
- Blessings-with-target: "In Kult → Prayers the offer row has the three choices" and the list of who can be named · screen description / duplicate of the prayer articles
- "Your force list shows what is laid on each troop" · screen description
- "It stacks with the Arm ... Later rounds, and troops that join" · duplicate of [[the-bulls-horns]]
- "In Keryx: rite --prayer minoan_potnia_walls_fall" · CLI detail (in [[the-walls-fall]])
- "Retired: The Thread Shows a Path ..." · history
- "In Keryx, rite --list shows your prayers, their effects" · screen description
- "keryx cult and keryx rite --list print the same figures" and the Offer-row Favours line · screen description
- The generated Favours table (about 400 words) is untouched and counts toward the cap.


## Moved anchors

None. Every anchor of the eight articles is still on a heading (the new test pins them); sections whose body was folded away kept a short body under the same heading.

## Welcome

"Six things to know" (not seven) with a "These can wait:" line. The seventh, "soldiers need bread and silver every game day", is NOT written because it is false: Spearmen, Galleys and Emporoi draw 0 silver ([[units]] "Drafting and supporting forces"; reference/code: only Bronze Guard, chariots and War Galleys draw pay). Reviewer: say if you want "soldiers eat every game day" instead. Checked: founding forecast ([[founding]]), starvation collapse below 50 ([[food]]), "timber almost always your bottleneck" (stated in [[goods]], design claim, not a code constant), copper/tin separation (`bronze.md`, mapgen landmass bias), Bronze Guard needs bronze + Foundry (reference.json). `welcome` anchor `welcome-your-opening-dispatch` kept.

## Contradictions with code or between articles (reported, not fixed beyond item 4)

- temples.md and kharis.md said a level-1 temple raises kharis by "0.25 per game day"; the generated table says 0.58 (0.25 is the grain tithe). Both sentences were cut as part of the clear-out; nothing was added in their place.
- rites.md said "each of the ten Minoan everyday prayers"; fourteen are listed. The count was dropped.
- Item 4 done: the "scarce in the world" clause is gone from kharis.md.
- Not verified line by line against code (kept as written): building queue of two waiting builds, workshop 3x speed, ship repair 8 % / 10-ship queue, runner/ship risks. Numbers in the eight leads were checked: wall ceilings and +4 workplaces and x1.7/2.4/3.1 (economy/population_cap.go, hex_rules.go), temple workplaces and ceilings and demolition 10 per level (reference.json kharis), foot terrain times (province/movement.go), runner half speed (province/pathfind.go).

## Not reached / notes

- All eight reached <= 60 %: runners 59.6 %, kharis 59.3 %, buildings 58.7 %, temples 59.8 %; the generated tables (kharis about 620 words, rites about 400, buildings about 250, temples about 270) cannot be touched and eat most of the budget, so the cuts in those four are deeper in the prose than the percentage suggests.
- Prayers in rites.md no longer repeat offerings and rest per prayer; the reader finds them in the generated table in kharis.md and on each prayer's own page. If you want them back in rites, that costs about 150 words.
- ship-types.md: the generated unit list moved under a new un-anchored heading "The three ships" so the lead stays short.
- `codex_links.mjs` added a few first-mention links while I worked; articles.json regenerated.
- Keryx Codex tests: `go test ./cmd/keryx/` green. Node: 37 pass (29 old + 8 new). `codex_facts.py --check` clean.

