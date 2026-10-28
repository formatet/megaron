Cargo follows a real route around impassable ground. Its path and the terrain costs
are fixed when each leg departs. Caravan time is **1.5 times the march time on that
same path**, rounded once to the nearest whole game day, with at least one game day per leg.
The return leg is priced separately because it enters different terrain. Cargo
quantity limits a ship's hold; it does not change journey speed.

Without a traversable road or an eligible ship on a sea lane, a shipment is refused
before goods are taken. A standing route pauses and explains the obstacle. The
arrival time shown in your cargo list is the scheduled delivery; delays or
interception can prevent that delivery.

In Keryx, `transfer` sends from your **selected city**, which may be a colony.
Check it with `keryx status`, or choose the sending city explicitly:
`keryx transfer --from Knossos --dest Mallia --good grain --qty 50`.
The sending city cannot also be the destination. A rejected self-transfer sends
nothing; use `--from` to choose the other city you meant to send from.
