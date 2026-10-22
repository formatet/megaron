# ORDER 16: client evidence

Keryx recruitment and status no longer describe returned garrison pay. Recruitment retains the compatible server net field for affordability, and current server gross/net amounts are equal. Status displays the server army_upkeep silver total. Unit roster reads upkeep_silver_per_tick directly.

Web Army cards and march forecasts already render the authoritative server silver values. No production JavaScript, CSS or markup changed, so no web BILD is triggered by this diff. Test fixtures verify elite home/field pay, chariots, actual reduced crews, repair and fitting-out pay; existing laid-up fixtures verify zero pay. Codex Upkeep, Units and Sea now explain the wage basis, half pay at home, crew-based pay and zero laid-up cost. Food remains ORDER 12.

Validation: 41/41 targeted Node tests (war_cards, march_preview, codex); `go test ./cmd/keryx -run 'TestUnitRosterUpkeepShowsZeroSilverAndAuthoritativeRations|TestMarch.*|Test.*Recruit.*|Test.*Upkeep.*' -count=1` passed. These client tests require no database. Server persistent integration, named red and mutation evidence belong to the main implementation report.

Pattern audit for circulation, 70 percent, old pay numbers and status-independent silver found no remaining player-facing circulation claim in web/Keryx/Codex. Unrelated 70-percent sun-radius comment remains unchanged.

Recruitment uses four significant figures for displayed food/pay and upkeep shortfalls, preserving an Emporos home wage of 0.05 and a reduced-crew shortage of 0.005. `TestRecruitPayKeepsSmallCrewWagesAndShortfalls` covers both the cost and one/both-resource warning branches.
