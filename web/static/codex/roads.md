A road is a built connection between neighbouring land hexes.

<!-- src: server/internal/combat/road.go, server/internal/province/roads.go -->
A **road** is a worn way between two neighbouring hexes, built by an army out of [[stone|stone]]. Once it stands, **everyone** who walks it travels faster — your people, your [[messengers|messengers]], your caravans, and your enemy's. It never decays and needs no [[upkeep|upkeep]].

## Road movement costs {#roads-what-a-road-does}

On a road, every hex you **enter** takes the same time **whatever the ground** — a hill or [[semi-desert|desert]] hex costs no more than a plain. Time per hex over a road edge:

| Traveller | Time per hex on a road |
|---|---|
| Foot soldiers | 0.75 game day |
| Runners and messengers | 0.35 game day |
| Caravans | 0.6 game day |
| [[war-chariot|Chariots]] | 0.4 game day |
| Ships | no effect |

(Off the road a foot soldier needs 1 game day on [[plains|plains]], 2 on [[hills|hills]] and 3 in semi-desert, so a road helps most where the ground is hardest.)

Roads join **land** hexes: plains, [[river-valley|river valley]], [[river-delta|river delta]], [[olive-grove|olive groves]], [[maquis|maquis]], hills, [[cedar-forest|cedar forest]] and semi-desert. They never cross water or mountains, and there are **no bridges** — a road ends at a [[rivers|river]] [[river-ford|ford]] and the ford is waded as before.

## Building a road {#roads-building-one}

**War → Forces → Build road** (or `keryx unit build-road --unit <id> --to q,r`) orders one army to build **one edge**, from the hex it stands on to a neighbouring hex. For a longer road, give another order.

The **stone** leaves your nearest city the moment you give the order — you are told at once if it has too little. It travels as a caravan to the first hex, and a [[runners|Runner]] carries the order to the army, which marches there. **Building starts only when both the stone and the army are at the first hex.**

| Ground the road reaches | Stone |
|---|---|
| Plains, river valley, delta | 8 |
| Olive grove, maquis | 12 |
| Hills, cedar forest | 16 |
| Semi-desert | 24 |

Three plains edges cost 24 stone. A hundred men build a plains edge in **1 game day** and a hill edge in **2**; fewer men take longer. When the edge is finished you get a **RoadEdgeBuilt** notice.

## Interrupted road construction {#roads-when-it-goes-wrong}

- The stone caravan is a physical caravan and **can be intercepted** ([[transfers]]). Then the road is not finished and the stone is lost.
- A **new order** to the army cancels its unfinished road. The army being destroyed or drawn into a [[battle]] before the edge is done ends it too. **Stone is never refunded.**
- A road still waiting for its stone or its army stays listed — `keryx roads` shows what each is waiting for.

## Seeing roads {#roads-seeing-roads}

You see a road edge only where your people have seen or remember **both** of its hexes. Unfinished roads of your own are drawn dotted.

Related: [[marching]], [[transfers]].
