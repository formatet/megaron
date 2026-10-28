`keryx places` lists known places, their kind and anchor id (`q,r`), their known hexes, and whether you may name them.

`keryx places name river 6,79 "Alpheios"` names a river. Use `massif`, `desert` or `forest` for the other kinds, with the id shown in your list. Add `--json` for the complete API reply. If an anchor starts with a minus sign, put `--` before the positional arguments: `keryx places name -- river -6,79 "Alpheios"`.

Administrators can change or remove unsuitable names with `keryx places rename <kind> <q,r> "<name>"` or `keryx places unname <kind> <q,r>` and their admin key. Removing a name leaves the naming right spent.

Storms follow their own rule: the first ship struck by a storm earns its naming right. See [[storms]].
