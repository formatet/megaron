# First hour, round 2 (newcomer playtest)

Window: desktop 1280x800, then phone 390x844. Player "Nestor2". Wall clock 01:05 to about 01:45, so roughly 20 game days. Screenshots in `shots/`, running notes in `log.md`.

## 1. What I managed
| Goal | Actions (approx.) | Result |
|---|---|---|
| Sign up, enter the world | 8 | OK. I was never asked for a ruler name; the top-left suddenly said "Wanax Komawes". |
| Understand where I am | 12 | OK thanks to the "?" Codex and the dispatch "Your people await you". |
| Choose a place and found a city | 8 | OK. I founded on the spot my Host already stood on. I did not weigh alternatives. |
| Understand food / hunger | 8 | Mostly. Food is safe, but the screens disagree in tone (see table). |
| Put people to work | 14 | OK, but fiddly. |
| Build something | ~25 incl. waiting | Farm queued after about 25 real minutes. |
| Send someone to look | 12 | Galley expedition worked well. Spearmen "explore" failed. |
| Find out what happened over time | 6 | Notifications and Economy show it. |
| Every main screen opened once | 12 | City, War, Diplomacy, Economy, Kult, Feedback, Codex, Notifications, Legend, Search. |
| Phone | ~35 | Look at map, tap hex, open city, change worker, queue view, march. Could not afford a new building. |

## 2. Where I got stuck (desktop)
| Trying to | Screen showed (exact words) | Expected | Did next | Severity | Screenshot |
|---|---|---|---|---|---|
| Build my first building | Every building "needs 8 timber, city has 0" or similar; timber +0.3 per game day, later +0.7 | To find out how to get timber faster | Moved workers around; found each hex has only ONE timber place; waited about 20 real minutes for a 6-timber Farm. Nothing on screen said timber is capped by the land. | LOST | 0039, 0046, 0102, 0105, 0149 |
| Rebalance workers | "Citizens: 10 of 10 placed ... 0 free" | A way to move a worker | Found by trial that I must press -1 on one hex, then +1 on another; the +1/-1 buttons also jump position after each click | LOST | 0030, 0048, 0051 |
| Pick the Farm site | Tap "CHOOSE A HEX ->" while "needs 4 stone, city has 0" | A disabled button | It was enabled and jumped me to the Production tab. After I picked a hex the drawer refreshed on its own, wiped my pending choice and scrolled to the top | CONFUSED | 0039, 0153, 0159 |
| Check if the Farm can start | Drawer said "needs 6 timber, city has 5" long after Economy said timber 6 | Same number everywhere | Drawer numbers lag; I had to leave and reopen City | CONFUSED | 0166 vs 0168 |
| Send Spearmen to look around | "Nothing sent - No route from this unit's current position to unseen ground within 5 hexes of (0,58) - try another area with reachable unseen ground." | The soldiers to scout | Unticked "Explore" and just marched to the hex | CONFUSED | 0065 |
| Understand "Destination city" | "Destination city: Choose a city..." drop-down on every march | To know if it is required | Ignored it; marching worked | CONFUSED | 0062 |
| Judge food | Warehouse: Grain "-5.9 per game day" in red; elsewhere "Food +0.5 per game day", "stocks growing" | One clear verdict | Read the "More" breakdown (fish +6 covers the grain). Red minus on grain alarms a newcomer. | CONFUSED | 0149, 0071 |
| Know if the clock runs | Dispatch: "Game days begin after 4 Wanaxes join." Start card: "FORMING / 0 Wanaxes (rulers)" | A clear yes/no | Waited; the clock did run | CONFUSED | 0017, 0010 |
| Read population | "In service 1,010", "People 1,000", "Walls hold 20", "Citizens 10", "Livestock 50" | One simple number | Worked out 1 citizen = 100 people from a small grey sentence further down | CONFUSED | 0033 |
| Name the cape | "Your unit was the first to reach this cape. Give it a name (once, 2-30 characters)" inside the Production tab, between stats | Something about my own city | Typed a name + Enter; no visible change, I do not know if it saved | CONFUSED | 0182 |
| Build a temple | Kult > Temples: four temples, "OPEN CITY TO BUILD", no cost, no explanation of the gods | To know what each does | Left | CONFUSED | 0075 |
| Look at the top bar | Notification strip squeezed into "Ty li", "x all", "Mission complete -" | Readable headlines | Opened the bell | ROUGH | 0129, 0137 |
| Understand the hammer button on the map | Red-highlighted button, a dotted marker near the city, no label | Something | Toggled it off | ROUGH | 0094 |
| Garrison tab | "DISBAND" beside the unit, "Returns 100 people" | A confirm or warning | Did not press | ROUGH | 0147 |
| Read arrival message | "Salt and Myrrh, Galley ... arrived at (2, 59) - garrison" | Home | I did not understand "garrison" or why (2,59) | ROUGH | 0185 |
| Panels overlap | Codex (left) and Notifications (right) open together, covering most of the map | One panel at a time | Closed both | ROUGH | 0140 |
| City drawer | A big picture takes the top third; the tabs and all controls are below it | Controls first | Scrolled every time | ROUGH | 0026 |

## 3. Words I did not understand
- **Wanax**, **Host**, **metropolis**: glossed on the start card and in the Codex, but my name "Komawes" appeared without me choosing it.
- **Kharis** ("Kharis 39.7 - Indifferent", "Wrath risk below 30", "Blessings from kharis 60") - no explanation on the Kult screen itself.
- **Catchment**, **Walls hold**, **In service**, **Livestock**, **herd**.
- **Rite**, **prayers**, **Runners**, **Lasts / not running down**, "Unavailable: ... [?]" lines on Economy, Kult and Garrison - I never learned what they were.
- **Garrison**, **stance: no stance**, **Set**, **Build road**, **Recall all**, "Retreat when it goes badly".
- **The Pithoi (1), Year 1** and "DAY 14 OF THE PITHOI" in the notices, next to "today 01:30" - two clocks.
- **Colonize** (phone) vs "Found a new settlement".
- "**#1 not yet scouted #2 not yet scouted #3 not yet scouted**" under the hex diagram.
- "**None of yours**" under every hex line.
- "**Full**" vs the dots (o o o o).

## 4. Phone (390x844)
| Trying to | Screen showed | Expected | Did next | Severity | Screenshot |
|---|---|---|---|---|---|
| Come back after resizing | Reload at phone size dropped me on the sign-in page | To stay in | Signed in again | ROUGH | 0189 |
| March across water | "Forecast unavailable: No passable route to (1,61) ... keryx unit load --ship <id> --unit <id> (ship and unit in the same settlement ...)" | A plain game message | Picked another hex | LOST | 0219 |
| Use the city drawer | Full screen; covers map and the bottom nav; hex diagram labels about 7px tall | To keep seeing the map | Scrolled, tapped hexes by guess (worked) | CONFUSED | 0199, 0201 |
| Buttons after tapping | +1/-1/FILL jump position after each tap | Stable buttons | Took a picture after each tap | CONFUSED | 0208, 0211 |
| March popup | Fills the whole screen, map not visible while choosing units | Small popup | Worked, then closed | ROUGH | 0219, 0223 |
| Queue/Buildings | "No buildings yet" and "Queue: 1/3" together; Farm shown as queued while I thought it done | Clear status | Read the queue list | ROUGH | 0214 |
| Top bar | Day name "THE PITH..." cut off | Full name | Ignored | ROUGH | 0195 |
| Pick a hex on the map | Bottom sheet (name, produces, March / Colonize) | -- | Worked well | -- | 0197 |

## 5. What worked well
- The "?" Codex: searchable, linked, and the first-steps dispatch tells you what to do in order. I never felt lost on screen one.
- Clicking a hex gives a forecast (food/timber/stone/copper/ground) BEFORE founding - I could read the land.
- The galley expedition card said exactly how long it takes and when it is home; the map visibly filled in.
- "needs 6 timber, city has 5" tells you what is missing and "ready = today 01:42" tells you when.
- The Legend explains the little worker figures.
- Search box (cities, armies, letters).

## 6. What I never found out
- Why my catchment gives so little timber and where good timber is; whether the city can grow beyond 20 citizens and how.
- What kharis does for me, what a temple costs, how to raise it.
- Where other players are; Diplomacy was empty ("No letters yet", "No runners ... of anyone else's that you know of"), so I could not test it.
- Whether naming the cape saved.
- What the hammer button and "stance" do.
- How to find tin (the forecast said "none seen") and whether the game wants me to.

## 7. The one thing
Make the first building reachable without 20 minutes of waiting and guessing: the whole first hour is gated by timber that the land supplies at about 0.7 per game day, and nothing on screen says so. A newcomer sees a row of greyed BUILD buttons, shuffles workers between hexes, and has no way to tell whether they are doing it wrong or whether the game is simply that slow.

Keep working, one thing at a time, until compute runs out.
