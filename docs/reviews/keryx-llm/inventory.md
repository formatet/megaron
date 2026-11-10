# keryx inventory (2026-11-10, master 612c269d)

Nothing is hidden today (`Hidden` is unused); 91 entries in root help incl. `help`, `completion`. "r/w": w = writes game state or account. "Hidden today" is `no` for every row. City naming flags: `province` = city name/id/settlement id/q,r via `resolveProvince`; `from` = acting city (name) on route/message/messenger/trade-offer/transfer; `settlement` = abandon/rite/occupation; `to`/`dest` = destination, not the acting city. A bare `[city]` in Usage = positional already accepted (build, city, placements, demolish, place, status, staff-as-first-arg none). Help words = `wc -w` of `--help`.

| command | aliases | hidden today | r/w | city naming flags | usage args | id flags | help words |
|---|---|---|---|---|---|---|---|
| abandon | - | no | w | settlement | [flags] | - | 79 |
| actions | - | no | r | province | [category] [flags] | - | 136 |
| agora | - | no | w | none | [flags] | - | 58 |
| army | - | no | r | none | [flags] | - | 36 |
| brief | - | no | r | none | [flags] | - | 31 |
| build | - | no | w | province | [city] [flags] | - | 191 |
| build-road | - | no | w | from,to | [flags] | unit | 240 |
| call-back | - | no | w | none | [flags] | id | 51 |
| cancel-build | - | no | w | none | [flags] | id | 43 |
| cargo | - | no | r | none | [flags] | - | 34 |
| cities | cities, wanaxes | no | r | none | [flags] | - | 151 |
| city | - | no | r | province | [city] [flags] | - | 129 |
| codex | - | no | r | none | [article] [flags] | - | 51 |
| demolish | - | no | w | province | [city] [flags] | - | 113 |
| diplomacy | - | no | r | none | [flags] | - | 30 |
| disband | - | no | w | province | [flags] | unit | 77 |
| dispatches | - | no | w | none | [flags] | - | 146 |
| dossier | - | no | r | none | [flags] | - | 114 |
| forces | forces, units | no | r | none | [flags] | - | 35 |
| founding | - | no | w | none | [command] | - | 73 |
| god | - | no | r | none | [flags] | - | 51 |
| goods | - | no | r | province | [flags] | - | 53 |
| gossip | - | no | r | none | [flags] | - | 26 |
| hex | - | no | r | none | <q,r> [flags] | - | 31 |
| idle | - | no | r | province | [flags] | - | 41 |
| inbox | - | no | r | none | [flags] | - | 29 |
| join | - | no | w | none | [flags] | - | 111 |
| kingdom-borrow-army | - | no | r | none | [flags] | - | 31 |
| kingdom-council | - | no | r | none | [flags] | - | 28 |
| kingdom-election | - | no | r | none | [flags] | - | 31 |
| kingdom-election-call | - | no | r | none | [flags] | - | 29 |
| kingdom-found | - | no | r | none | [flags] | - | 28 |
| kingdom-invitations | - | no | r | none | [flags] | - | 28 |
| kingdom-invite | - | no | r | none | [flags] | - | 30 |
| kingdom-join | - | no | r | none | [flags] | - | 31 |
| kingdom-treasury-deposit | - | no | r | none | [flags] | - | 30 |
| kingdom-vote | - | no | r | none | [flags] | - | 32 |
| kingdoms | - | no | r | none | [flags] | - | 26 |
| leave | - | no | w | none | [flags] | - | 68 |
| letters | - | no | r | none | [wanax] [flags] | - | 26 |
| login | - | no | w | none | [flags] | - | 98 |
| map | - | no | r | province | [flags] | - | 71 |
| march | - | no | w | to | [flags] | unit | 1109 |
| market-board | - | no | w | none | <city> [flags] | - | 165 |
| me | - | no | r | none | [flags] | - | 29 |
| message | - | no | w | from,to | [flags] | - | 112 |
| messenger | - | no | w | from,to | [flags] | message | 109 |
| notifications | - | no | r | none | [flags] | - | 89 |
| occupation | - | no | w | none | [command] | - | 57 |
| outbox | - | no | r | none | [flags] | - | 29 |
| passage | - | no | w | none | [flags] | id | 78 |
| password | - | no | w | none | [flags] | - | 47 |
| place | - | no | w | province | [city] <good> <ordinal> [+n/-n] [flags] | - | 159 |
| placements | - | no | r | province | [city] [flags] | - | 129 |
| places | - | no | r | none | [flags] | - | 105 |
| recall | - | no | w | none | [flags] | unit | 112 |
| recruit | - | no | w | province | [flags] | unit | 143 |
| redirect | - | no | w | to | [flags] | unit | 205 |
| reinforce | - | no | w | none | [flags] | unit | 82 |
| reply | - | no | w | none | [flags] | id | 38 |
| report | - | no | w | none | [flags] | - | 91 |
| reports | - | no | r | none | [flags] | - | 46 |
| retreat-default | - | no | w | none | [flags] | - | 138 |
| rich-offering | - | no | w | province | [flags] | - | 92 |
| rise | - | no | w | none | [flags] | - | 106 |
| rite | - | no | w | province,settlement | [flags] | offer | 295 |
| roads | - | no | r | none | [flags] | - | 89 |
| roster | - | no | r | none | [flags] | - | 67 |
| route | - | no | w | from,to | [flags] | - | 119 |
| route-delete | - | no | w | none | [flags] | id | 48 |
| route-pause | - | no | w | none | [flags] | id | 44 |
| route-resume | - | no | w | none | [flags] | id | 40 |
| routes | - | no | r | none | [flags] | - | 28 |
| runners | - | no | r | none | [flags] | - | 33 |
| settlements | - | no | r | none | [flags] | - | 28 |
| sightings | - | no | r | none | [flags] | - | 108 |
| staff | - | no | w | province | <byggnad> [+n/-n] [flags] | - | 169 |
| status | - | no | r | province | [city] [flags] | - | 131 |
| storms | - | no | r | none | [flags] | - | 170 |
| ticklog | ticklog, journal | no | r | province | [flags] | - | 72 |
| trade-accept | - | no | w | none | [flags] | id | 31 |
| trade-cancel | - | no | w | none | [flags] | id | 44 |
| trade-decline | - | no | w | none | [flags] | id | 31 |
| trade-offer | - | no | w | from,to | [flags] | message | 222 |
| transfer | - | no | w | province,from,dest | [flags] | - | 162 |
| unit | - | no | r | none | [command] | - | 223 |
| unit build-road | - | no | w | from,to | [flags] | unit | 241 |
| unit cancel-training | - | no | w | none | [flags] | unit | 42 |
| unit fit-out | - | no | w | none | [flags] | unit | 47 |
| unit lay-up | - | no | w | none | [flags] | unit | 50 |
| unit list | - | no | r | none | [flags] | - | 33 |
| unit load | - | no | w | none | [flags] | unit | 51 |
| unit march | - | no | w | to | [flags] | unit | 1110 |
| unit patrol | patrol, sentry | no | w | none | [flags] | unit | 117 |
| unit pickup | - | no | w | none | <unit> [flags] | - | 83 |
| unit post | - | no | w | to | [flags] | unit | 219 |
| unit recall | - | no | w | none | [flags] | unit | 113 |
| unit redirect | - | no | w | to | [flags] | unit | 206 |
| unit reinforce | - | no | w | none | [flags] | unit | 83 |
| unit repair | - | no | w | none | [flags] | unit | 100 |
| unit retreat-order | - | no | w | none | [flags] | unit | 108 |
| unit stance | - | no | w | none | [flags] | unit | 155 |
| unit unload | - | no | w | none | [flags] | - | 41 |
| version | - | no | r | none | [flags] | - | 29 |
| watch | - | no | r | none | [flags] | - | 155 |
| worlds | - | no | r | none | [flags] | - | 35 |

Notes: `ids demanded` in the id-flags column = the flag is required today (call-back, cancel-build, trade-accept/decline/cancel, reply, passage, route-pause/resume/delete). `gift` is not its own verb (it is `transfer`); `cult` is not a command (it is data inside `dossier`/`actions`), `stance/load/unload/repair/cancel-training` exist only as `unit` subcommands. `unit` subcommands marked above.
