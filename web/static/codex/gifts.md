A **gift or tribute** sends goods or [[silver|silver]] to another Wanax's city, with nothing requested in return. It needs no acceptance. The city must have a known location on your map, through sight, exploration or completed contact. A rumour alone is not enough.

Open **Economy → Transfer**. Choose your sending city, a destination marked with its Wanax's name **(gift)**, one or more goods and their quantities, then **Send →**. In Keryx, use `keryx transfer --good silver --qty 20 --dest CityName`; use the settlement ID if two cities share a name. Your own cities remain [[transfers|internal transfers]].

## Gifts in transit {#gifts-on-the-road}

The goods leave your stock when sent. They travel in a physical caravan, or aboard a real free ship from your port when a sea route is used. Cargo can be intercepted. A ship remains bound until its empty voyage home finishes. A gift carries the same risk as any other cargo: on land only raiders, at sea also [[sea|storms]].

A ship damaged in interception keeps the existing [[transfers|naval interception outcome]]: half of each good is lost, half sails home. The gift notice separates the exact quantities **lost** and **sent home**. This is a physical damaged return, not an immediate refund; that voyage can itself be intercepted.

## Receiving gifts {#gifts-at-the-destination}

The city receives only what fits in its store. Anything that does not fit is lost. **GiftDeliveredV2** or **GiftLostV2** tells you the exact quantities received and lost. There is no return of goods.

If the city changes hands during the journey, its **new owner receives the gift**. The sender, intended recipient and actual recipient are notified. If the city has fallen or has no owner, the gift is lost.

A gift appears as a row among the letters in **Diplomacy → Threads**. The sender can see its departure; the recipient sees the outcome when delivery is resolved. It creates no [[trade|trade offer]] and asks for nothing in return. Notifications and Keryx show the same delivery outcome.

To send goods between your own cities, use an [[transfers|internal transfer]].

The live cargo meter uses the same limits as an internal transfer: **[[galley]] 20 / [[emporos]] 120 goods**, one place per good of any kind. Several goods share one voyage.
