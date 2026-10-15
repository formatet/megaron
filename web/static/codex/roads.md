<!-- src: server/internal/combat/road.go, server/internal/province/roads.go -->
A **road** is a worn way between two neighbouring hexes, built by an army out of stone. Once it stands, **everyone** who walks it travels faster — your people, your messengers, your caravans, and your enemy's. It never decays and needs no upkeep.

## What a road does

A road makes the hex you **enter** over it quicker:

| Traveller | Time over a road edge |
|---|---|
| Foot soldiers | 75% |
| Runners and messengers | 70% |
| Caravans | 60% |
| War chariots | 60% (of their own, faster time on flat ground) |
| Ships | no effect |

Roads join **land** hexes: plains, river valley, river delta, olive groves, maquis, hills, cedar forest and semi-desert. They never cross water or mountains, and there are **no bridges** — a road ends at a river ford and the ford is waded as before.

## Building one

**War → Army → Build road** (or `keryx unit build-road --unit <id> --to q,r`) orders one army to build **one edge**, from the hex it stands on to a neighbouring hex. For a longer road, give another order.

The **stone** leaves your nearest city the moment you give the order — you are told at once if it has too little. It travels as a caravan to the first hex, and a [[runners|Runner]] carries the order to the army, which marches there. **Building starts only when both the stone and the army are at the first hex.**

| Ground the road reaches | Stone |
|---|---|
| Plains, river valley, delta | 8 |
| Olive grove, maquis | 12 |
| Hills, cedar forest | 16 |
| Semi-desert | 24 |

A stone wall costs 24 stone, as much as three plains edges. A hundred men build a plains edge in **1 day** and a hill edge in **2**; fewer men take longer. When the edge is finished you get a **RoadEdgeBuilt** notice.

## When it goes wrong

- The stone caravan is a physical caravan and **can be intercepted** ([[transfers]]). Then the road is not finished and the stone is lost.
- A **new order** to the army cancels its unfinished road. The army being destroyed or drawn into a [[battle]] before the edge is done ends it too. **Stone is never refunded.**
- A road still waiting for its stone or its army stays listed — `keryx roads` shows what each is waiting for.

## Seeing roads

You see a road edge only where your people have seen or remember **both** of its hexes. Unfinished roads of your own are drawn dotted.

Related: [[marching]], [[transfers]].
