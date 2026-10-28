Winning at a city — by [[battle]] or by [[sieges|siege]] — does not make it yours. It becomes **occupied**: still owned by its Wanax, garrisoned by the victor's survivors, its production frozen.

**Hold it for 30 game days without a fight and you may annex it.** **Any new fighting at the city starts the count again.** The defender can send a relief force.

## Completing an occupation {#occupation-when-the-occupation-ripens}

You do not have to wait for the full count. As soon as the city falls you may:

- **sack** it. Your men carry off half its [[silver|silver]] and a share of its other goods (you may choose which), its best production building loses a level, and a third of its people are lost (it never falls below 50). The city stays theirs. The loot goes home as a caravan, which can be intercepted on the way like any other ([[transfers]]), and your army leaves the city;
- **burn** it — sack it, then raze it to the ground. The city burns on the map for the rest of that game day, for anyone who has eyes on it, and stands as a ruin after. **Nobody may found a city there for 60 game days.**

After **30 game days unchallenged** you are told the city is ripe, and you may also:

- **annex** it — keep it as your own. It becomes one of your cities, never your capital. Ships in its harbour change hands with it.

Until you choose, the city remains occupied.

Make the choice from the dispatch that told you about it — sack/burn/annex are buttons right
there, no need to leave the browser — or from the command line with `keryx occupation` ([[keryx]]).

## Relieving an occupied city {#occupation-when-it-happens-to-you}

You are notified when your city is occupied, when an occupation is broken, and — if it comes to that — when it is sacked, looted, burned or taken. A relief force is your answer: every fight at the city resets the occupier's countdown.

If the city you lose was your last, see [[last-city]].

## Losing your capital {#occupation-capital-succession}

If your capital is annexed, razed or collapses while you still hold another city, your **most populous** remaining city becomes your capital at once; there is no choice to make. <!-- src: server/internal/combat/succession.go handleOwnerCityLoss -->

<!-- src: server/internal/combat/occupation.go occupationTicksToAnnex, sackPopLossFraction, burnRecolonizeKaren -->

