# keryx `march --help`: 94 lines to 30

Order: `.agents/order-keryx-march-help-20261117.md`.

- `march --help` (and `unit march --help`, same builder) went from 94 to 30 lines. Output before and after:
  `help-before.txt`, `help-after.txt`.
- The long form (all the removed prose and the 12 old examples, 77 lines) lives in `keryx help march`
  (`help-long.txt`). The last line of the short help names it. Mechanism: a command may carry
  `Annotations["long"]`; the replaced `help` command (`longHelpCmd` in `cmd_help.go`) prints it. Other
  commands behave as before. The terrain pointer (`keryx codex marching`) and storms pointer stay in the short help.
- Flag descriptions shortened to one line each. No flag, behaviour or JSON change.
- `help` no longer needs a login (added to the PersistentPreRunE exemption in `main.go`; needed so the
  test can run `help march` with no config).
- Recall/redirect are separate commands, not owned by `march`, so they have no example here.
- Test: `cmd_march_help_test.go` (<= 30 lines, names every visible flag read from the flag set, last line is the
  pointer, `help march` carries the long form). Red before (94 lines), green after. Two older tests that read
  `Long` were left as is and still pass.

## `--help` length per command (lines, after)

| lines | command |
|---|---|
| 38 | founding settle |
| 35 | rite |
| 31 | unit |
| 31 | occupation order |
| 30 | unit march |
| 30 | march |
| 30 | actions |
| 29 | unit build-road |
| 29 | market-board |
| 29 | build-road |
| 28 | trade-offer |
| 28 | place |
| 27 | watch |
| 27 | status |
| 27 | dispatches |
| 26 | unit post |
| 26 | retreat-default |
| 25 | placements |
| 25 | city |
| 25 | build |
| 24 | cities |
| 23 | recruit |
| 23 | notifications |
| 22 | unit stance |
| 22 | unit redirect |
| 22 | unit patrol |
| 22 | transfer |
| 22 | storms |
| 22 | staff |
| 22 | redirect |
| 21 | unit recall |
| 21 | recall |
| 21 | messenger |
| 21 | login |
| 21 | join |
| 21 | dossier |
| 20 | ticklog |
| 20 | route |
| 20 | rise |
| 20 | message |
| 20 | demolish |
| 19 | unit retreat-order |
| 19 | rich-offering |
| 19 | report |
| 19 | places |
| 18 | unit repair |
| 18 | unit reinforce |
| 18 | sightings |
| 18 | roads |
| 18 | reinforce |
| 17 | passage |
| 17 | map |
| 17 | forces |
| 17 | disband |
| 17 | abandon |
| 16 | unit pickup |
| 16 | reply |
| 16 | founding |
| 16 | cancel-build |
| 16 | call-back |
| 16 | agora |
| 15 | unit load |
| 15 | trade-decline |
| 15 | trade-cancel |
| 15 | trade-accept |
| 15 | route-resume |
| 15 | route-pause |
| 15 | route-delete |
| 15 | reports |
| 15 | occupation |
| 15 | leave |
| 15 | goods |
| 15 | god |
| 15 | codex |
| 14 | unit unload |
| 14 | unit list |
| 14 | unit lay-up |
| 14 | unit fit-out |
| 14 | founding status |
| 14 | army |
| 13 | routes |
| 12 | roster |
| 11 | worlds |
| 11 | unit cancel-training |
| 11 | storms unname |
| 11 | storms rename |
| 11 | places unname |
| 11 | places rename |
| 11 | password |
| 11 | inbox |
| 11 | idle |
| 10 | version |
| 10 | storms name |
| 10 | settlements |
| 10 | runners |
| 10 | places name |
| 10 | outbox |
| 10 | me |
| 10 | letters |
| 10 | kingdom-vote |
| 10 | kingdom-treasury-deposit |
| 10 | kingdoms |
| 10 | kingdom-join |
| 10 | kingdom-invite |
| 10 | kingdom-invitations |
| 10 | kingdom-found |
| 10 | kingdom-election-call |
| 10 | kingdom-election |
| 10 | kingdom-council |
| 10 | kingdom-borrow-army |
| 10 | hex |
| 10 | gossip |
| 10 | diplomacy |
| 10 | cargo |
| 10 | brief |
| 10 | agora password |

Root `keryx --help`: 38 (separate map, test-capped at 45). No command other than `march` is over 40, so no
follow-up is listed; the longest are `founding settle` (38), `rite` (35), `unit` (31).
