# Kharis TEXT-test — ORDER29

rigg: commit 7f0cdeb · migration 203/203 · värld 1312f0dd · tick 5282 · TICK_SECONDS=3600 · 2026-10-23 06:07

Keryx: `/home/tk/.local/share/megaron-texttest/keryx`. Configs: `/home/tk/.config/poleia-texttest/<namn>.json` (mode0600).

Phoinix: faktisk worker-hanterad KharisTick, id 678114. Thersites: tillåten konsistent state/notis-fixtur. Syntetiska acceptansstäder; ORDER29-klienten har nya texter, serverreglerna är oförändrade. Riggen lämnas igång.

## Eumelos

```text
The gods are Indifferent (kharis 40.0).

Offering values — per unit, before each god's preferences; not trade prices:
  bronze       15.53
  cedar        1.412
  copper       1.412
  fish         1.079
  grain        1.156
  incense      5.646
  livestock    1.12
  oil          1.412
  silver       1.122
  stone        2
  timber       1.111
  tin          1.412
  wine         1.412

Prayer ID                     Name                  MinKhar   Offering                Effect                                         Ready         Cooldown
minoan_potnia_vision_v2       Potnia's Vision       5         grain×10 silver×3       Marks an unseen ore deposit and the unknown ground within two hexes as memory; no live activity.  cooldown      24 days remaining
    Potnia favours fish ×2.2, incense ×2.1, purple ×2.0, oil ×1.5  ·  expects an offering worth ~12 (--offer good=amount)
minoan_potnia_walls           Mistress of the Walls  5         grain×8 timber×4        Your own garrison defending this city fights a quarter stronger for 6 days.  active        6 days remaining active; 24 days remaining wait
    Potnia favours livestock ×2.0, wine ×1.7, horses ×1.6, incense ×1.5  ·  expects an offering worth ~14 (--offer good=amount)
minoan_velchanos_green_shoots  Green Shoots          30        grain×15 silver×2       Grain, livestock, oil and wine yield a quarter more for 25 days; fish is unchanged.  no: no Temple of Velchanos  —
    Velchanos favours incense ×2.0, wine ×2.0, oil ×1.6, grain ×1.3  ·  expects an offering worth ~25 (--offer good=amount)
minoan_asterion_thread        The Thread Shows a Path  5         silver×3 timber×4       Your next land caravan or Runner from this city travels faster; the caravan is harder to intercept.  no: no Temple of Asterion  —
    Asterion
minoan_stillness_passage      Favourable Passage    5         fish×6 silver×3         The next ship leaving this port sails faster and takes no storm damage on its outward leg.  no: no Temple of the Sea Daimons of Stillness  —
    The Sea Daimons of Stillness

Kharis is a requirement, never spent. Everyday prayers answer when their requirements are met; legacy prayers may remain silent. An accepted offering is spent either way.
```

## Phoinix

```text
1 notification(s), 1 unread
────────────────────────────────────────────────────────────
*[today 05:49]  KharisDeclining
      Kharis is falling (29.5, net -2.50 per day). At this rate it reaches 15 in 6 days. Below that the gods may punish you, with or without a temple. Staff and feed your temples.
```

## Thersites

```text
Wanax Thersites · today 05:49

1 citizen = 100 people
Thersites 1 [minoan]  Pop: 1.0k people  Labor: 10 citizens  Walls: 0/3  Settlements: 1/5
Walls hold 2.0k. The next wall needs a Granary of level 1 (yours: 0) — `keryx build --type silo`.
Population 1000 — not growing: food net -5.0 per day (needs more than 0)

Food for 620.0 days at current consumption (100 grain, 1000 fish, 1000 oil, 1000 wine)
Herd 1000 / 50 — full: new animals are eaten, then lost. A pen raises the cap.

Last today 05:49: 0 goods produced, 0 consumed  ·  keryx ticklog for details

Resources
  (per day; food and army balances are itemized below)
  Silver     1.0k  —
  Food     production 0.0 − people eat 5.0 = net -5.0 per day (before army)
  Grain       100  production 0.0 per day  (5 citizens needed for people on best food slots · 0 on food)
  Automatic food: 0.50 grain + 0.00 fish per day without placed citizens (nearby ground 0.50 grain; 0 people outside whole citizens: 0.00 grain + 0.00 fish, a fraction of the best food worker)
  Army     eats 0 food per day → food net -5.0 per day after army
  Silver   net +0.0 per day after army pay
  Fish       1.0k  —
  Timber      990  —
  Stone      1.0k  —
  Copper     1.0k  —
  Tin        1.0k  —
  Bronze     1.0k  —
  ⚠ Timber production ~0 — timber gates harbour (16)/barracks (8)/foundry (8)/temple (8). Place workers on a timber hex: `keryx place timber <ordinal>` (see `keryx city`).
  Kharis       12  (Suspicious) · ceiling 50 · net -0.2 per day (temple − decay)
  Upkeep of the cult: 0.10 base + 1 city cost 0.00 + 1000 residents cost 0.10 = 0.20 kharis per day
  Temple production: 0.00 kharis per day
  One more fed temple worker gives at least +1.05 kharis per day, and more when the cult’s reference offering of grain is scarce in the world. Requires a free citizen, a temple workplace and its daily offering.
  Everyday prayers work at a level-1 temple. Spontaneous blessings need 60 kharis and a maintained temple. Only a level-2 temple or higher lets kharis grow that far. Kharis retained above a lowered ceiling still counts.
  Below 15 kharis: 3% chance of divine punishment per day, with or without a temple.
  Protected from divine punishment until today 12:49.
  → ceiling 50 is set by your level-1 temple; kharis is declining. Your temple has empty workplaces: assign a free citizen with `keryx staff temple +1` and supply its daily offering.
  Temple of Potnia in Thersites 1: reference 0.25 grain per day — alternatives: 0.2677 fish or 0.2601 timber or 0.2575 silver — stock: 100 grain, 1000 fish, 990 timber, 1000 silver — next gift: 0.25 grain  ✓
  → feed the temples with local grain, fish, timber or silver or cast a rite — see `keryx rite --list`.

Army (garrison in Thersites 1)
  Field units and other cities are shown in `keryx unit list`.

Buildings
  Temple of Potnia L1
```

## Laertes

```text
Wanax Laertes · today 05:49

1 citizen = 100 people
Laertes 1 [minoan]  Pop: 1.0k people  Labor: 10 citizens  Walls: 0/3  Settlements: 1/5
Walls hold 2.0k. The next wall needs a Granary of level 1 (yours: 0) — `keryx build --type silo`.
Population 1000 — not growing: food net -4.4 per day (needs more than 0)

Blessed harvest: +25% grain, oil, wine, livestock yield · 25 days left
Food for 798.0 days at current consumption (989.7525 grain, 1000 fish, 1000 oil, 1000 wine)
Herd 1000 / 50 — full: new animals are eaten, then lost. A pen raises the cap.

Last today 05:49: 1 goods produced, 0 consumed  ·  keryx ticklog for details

Resources
  (per day; food and army balances are itemized below)
  Silver     1.0k  —
  Food     production 0.6 − people eat 5.0 = net -4.4 per day (before army)
  Grain       990  production 0.6 per day  (5 citizens needed for people on best food slots · 0 on food)
  Automatic food: 0.50 grain + 0.00 fish per day without placed citizens (nearby ground 0.50 grain; 0 people outside whole citizens: 0.00 grain + 0.00 fish, a fraction of the best food worker)
  Army     eats 0 food per day → food net -4.4 per day after army
  Silver   net +0.0 per day after army pay
  Fish       1.0k  —
  Timber      990  —
  Stone      1.0k  —
  Copper     1.0k  —
  Tin        1.0k  —
  Bronze     1.0k  —
  ⚠ Timber production ~0 — timber gates harbour (16)/barracks (8)/foundry (8)/temple (8). Place workers on a timber hex: `keryx place timber <ordinal>` (see `keryx city`).
  Kharis       80  (Favorable) · ceiling 50 · net -0.2 per day (upkeep only above ceiling)
  Upkeep of the cult: 0.10 base + 1 city cost 0.00 + 1000 residents cost 0.10 = 0.20 kharis per day
  Temple production: 0.00 kharis per day
  Everyday prayers work at a level-1 temple. Spontaneous blessings need 60 kharis and a maintained temple. Only a level-2 temple or higher lets kharis grow that far. Kharis retained above a lowered ceiling still counts.
  Below 15 kharis: 3% chance of divine punishment per day, with or without a temple.
  → kharis declines through daily cult upkeep until it is below the ceiling 50; temple workers and offerings do not raise it there.
  Temple of Potnia in Laertes 1: reference 0.25 grain per day — alternatives: 0.2677 fish or 0.2601 timber or 0.2575 silver — stock: 989.8 grain, 1000 fish, 990 timber, 1000 silver — next gift: 0.25 grain  ✓

Army (garrison in Laertes 1)
  Field units and other cities are shown in `keryx unit list`.

Buildings
  Temple of Potnia L1
```

## Antilochos

```text
The god Potnia is angered: demolition costs −20 kharis now (45 → 25; floor 1). Ceiling 75 → 40. The lower ceiling takes no additional kharis. Above it, kharis declines only through daily cult upkeep; temple workers and offerings do not raise it.
Not demolished. Run again with --yes to confirm.
```
