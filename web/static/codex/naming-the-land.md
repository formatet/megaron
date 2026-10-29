The first Wanax whose **own physical unit reaches** a [[rivers|river]], mountain massif, [[semi-desert|desert]] or [[cedar-forest|cedar forest]] gains the right to name it. Seeing the place nearby gives no right. You receive a notice naming the unit that reached it first.

For a river, desert or cedar forest, your unit must enter one of its hexes. For a mountain massif, your unit must enter **land directly bordering the mountains**. Mountains remain impassable. If that land borders two separate massifs, you gain the right to name both. Simply seeing mountains from farther away gives no naming right.

## Naming known land {#naming-the-land-details}

A river includes its [[river-ford|fords]] and connected [[river-delta|delta]]. A mountain massif joins neighbouring limestone and [[red-mountains|red mountains]]; a desert joins neighbouring desert hexes; a cedar forest joins neighbouring [[cedar|cedar]] woodland. Massifs, deserts and cedar forests must cover at least five hexes to be named. Olive groves are cultivated land and cannot be named this way.

## Giving a place a name {#naming-the-land-give-it-a-name}

Click any known hex of the place on the map. If the right is yours, its panel offers a name box. The right lasts until used, with no time limit. **You may name it once.** A nameless place keeps its [[terrain|terrain]] title and has no name label on the map.

Names are trimmed, 2–30 characters, and may contain letters, digits, spaces, apostrophes and hyphens. A name must be unique in the world, ignoring letter case. Naming is immediate: it sends no order or [[runners|Runner]].

After naming, the name appears in the hexpanel and on the map at readable zoom. The TERRAIN row still describes the ground. Only hexes you can see or remember reveal the place and its name; naming never reveals unknown land. See [[sight]].

## Naming places in Keryx {#naming-the-land-keryx}

`keryx places` lists known places, their kind and anchor id (`q,r`), their known hexes, and whether you may name them.

`keryx places name river 6,79 "Alpheios"` names a river. Use `massif`, `desert` or `forest` for the other kinds, with the id shown in your list. Add `--json` for the complete API reply. If an anchor starts with a minus sign, put `--` before the positional arguments: `keryx places name -- river -6,79 "Alpheios"`.

Administrators can change or remove unsuitable names with `keryx places rename <kind> <q,r> "<name>"` or `keryx places unname <kind> <q,r>` and their admin key. Removing a name leaves the naming right spent.

Once a place has a name, anyone who has seen it can tell others of it in a letter. See [[telling-of-places]].

Storms follow their own rule: the first ship struck by a storm earns its naming right. See [[storms]].
