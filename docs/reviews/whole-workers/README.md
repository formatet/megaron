# Whole citizen groups in Population

Timothy: 1005 people must not appear as 5 free workers; only whole groups of 100
are shown as citizens/free to work. This changes presentation only. Raw population
and existing remainder food production remain unchanged. Primary population rows
show floor(population/100), floor(idle_citizens/100); exact people move to More.
Growth summary keeps its authoritative state/net without repeating raw population.

Validation: named remainder cases 1005/5→10/0,1099/99→10/0,1100/100→11/1,1205/205→12/2 RED on original renderer, GREEN after; all681JS GREEN. Exact population remains in More. No backend, idle allocation or rest-population production change.
