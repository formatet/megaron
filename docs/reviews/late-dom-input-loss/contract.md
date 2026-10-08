# Late DOM input loss — contract

Base: master 4895617b. One client bug-hunt slice, no design/server/API changes.
Proves the gate: preserve a player's unfinished orders while asynchronous data arrives.

Before production changes, exercise actual exported handlers in Chromium with
controlled delayed HTTP replies and a real DOM. Begin with City Garrison and War
Recruit. Record each observed loss separately; fix only proven losses. Inventory
all production innerHTML assignments, including writes in later callbacks and
writes whose awaited work is on previous lines. Classify the rest with a reason.

Acceptance: typed values, selected options, focus/text selections and grid
selection survive unrelated late responses. Invalid/removed server options are
never recreated. Existing explicit navigation/submission resets remain explicit.
Requests, capabilities, FOW gates, HTML/CSS/player text and server remain unchanged.
Physical mutations restore each proven bug and must fail its named reproduction;
restore source and run green. Run relevant tests, complete JS suite, fresh PG16 Go
suite and clean-environment vet. Real isolated game browser evidence accompanies
handler fixtures; visible evidence marked BILD. Handoff hash and stop.
