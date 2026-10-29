# Six Codex gaps, sequential changes

Contract: expose existing movement rules and prevent numerical drift, without changing game mechanics. Supports planning the geography-to-bronze progression. Each part committed and integrated in order: land terrain costs; ship repair price/time; occupation/razing timing; growth/meal variety; naval seizure outcomes; water travel rates.

Tests read actual domain constants/functions and player text. All six pass. One corrupted article value per part produced its named assertion failure, then restored text passed. Codex/catalogue and player-time checks: 29/29. Water article autolinks initially failed the value guard; explicit readable labels corrected this, and the final complete check passed. No production Go, migrations, or gameplay state changed. Terrain production yields remain a separate todo.
