**Gossip (🗣)** is what travellers, merchants and soldiers say about the world beyond your sight — who rules where, what happened, which cities exist.

Rumour is useful but blurred:

- it tells you **that** a city exists and **roughly** where, deliberately imprecise;
- it can be old news;
- it does **not** give you [[contact]] — you still have to reach a city before you can write to it or trade with it.

## Discovering and passing rumours {#gossip-details}

Rumours appear in the same list as your notifications, newest first, each marked **Rumour** with a 🗣 so you can tell what you have only heard from what happened to you. The rulers you know are listed under **Known** in Diplomacy.

Use gossip to decide where to send a [[messengers|messenger]] or a scout; use [[sight]] and [[contact]] to act on it.

## What becomes news, and how far it travels {#gossip-how-rumours-spread}

**Great events** — a city [[occupation|occupied]], annexed, sacked, burned, starved into surrender or collapsed — become rumours that the rulers nearby hear. **Small events** — a mine opened, a [[colonies|colony]] founded, goods flowing between two cities — are told only by those who saw them.

News moves with people. When your [[messengers|messenger]] reaches another ruler's city, you hear what that ruler has heard. Great news passes from ruler to ruler, at most **four** tellings from where it happened. Small news passes only from the eyewitness, once; whoever hears it second-hand does not pass it on. You hear each rumour at most once, however many ways it reaches you, and old news stops travelling. News received more than 72 real hours ago is no longer passed on. <!-- src: server/internal/gossip/gossip.go PropagateOnContact, maxMajorHops -->

## True divine omens {#gossip-divine-omens}

An omen is a true observation when it is given, rather than a promise about the future. It may point to a [[storms|storm]], metal-bearing ground, or the current balance between food production and the townsfolk's meals. When there is no true sign to convey, [[gods|the gods]] remain silent. Later movement or work can change what was observed.

Higher [[kharis]] brings signs more often, from farther away, and with more detail. Below 5 the minimum gap is 12 game days; at 5, 30, 60 and 90 it falls to 8, 5, 3 and 2 game days. These are minimum gaps, not promises that a sign will appear. A modest temple cannot reach the highest standing: a level 3 temple can support Kharis 100. Near Kharis 90 or above, a rare urgent coastal storm omen can name a concrete hex; it never gives an exact force, future arrival time or route. Low standing never reveals that precise hex. Names of unknown cities are not revealed.

Omens have their own star marker in Notifications. Your own goods movements are not told back to you as trade rumours.
