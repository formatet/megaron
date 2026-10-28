An offer’s expiry is the deadline for accepting or declining it.

New offers stay open for **24 game days after arrival**, and their runner waits for that same window. Offers already sent keep their recorded deadline. **Diplomacy → Letters** shows the reply deadline on your local clock; Keryx `outbox` shows when a pending offer expires.

No dispatch announces an expiry. At the deadline the offer lapses, its goods or silver are released back to your stores, and the letter reads *Expired without answer*.

<!-- src: server/internal/messenger/handler.go OfferExpiryTicks; saved ScheduledOfferExpiry deadlines remain unchanged -->

See [[trade-answering-an-offer]], [[trade]] and [[runner-return]].
