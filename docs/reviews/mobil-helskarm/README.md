# ORDER 11 — mobile fullscreen proof

Mobile panels occupy the screen, close the previous panel through its own
cleanup, and return map input when closed. Builds on rescued WIP d61992a5
(base/master d6b0032c). Search continuation e8f4f900 makes the closing branch
explicit. No merge, push or deployment. Proves mobile access in the chain gate;
**BILD remains for Timothy before merge**.

## Results

- 713/713 JS tests and `go vet ./...` GREEN. No server changes; the earlier
  full fresh Go run on port 57900 is preserved in `full-go.log`, not rerun.
- Search leak reported in review did **not** reproduce: d61992a5 already calls
  closeSearch in its else branch. New actual-module narrow-width test GREEN
  on rescued code, physical missing-cleanup mutant RED, exact restore GREEN:
  `search-mutation.log`. All registered hiding writes audited in
  `closing-audit.md`; no other missing cleanup found.
- Firefox 390×844, device scale 1, `has_touch=True`, actual `tap()` for player
  buttons and map. Eight scenes × two independent browser contexts per width
  (390 and 1000), for baseline and fix: **64 captures, 32 identical pairs**,
  zero page errors. Run hashes in `firefox.json` / `baseline-firefox.json`.
- Desktop 1000: **zero changed pixels in all eight baseline/fix pairs**;
  `comparison.json`. Mobile checks assert exactly one fullscreen panel,
  (0,0,390,844) bounds, inert canvas, reachable header after body scroll,
  predecessor cleanup and restored map touch. Extra close paths are exercised
  after the scene captures, with no panel left registered.

## Determinism investigation

Reproduced the original desktop-city failure before changing capture timing.
Pixel diff: 3,401 changed pixels, bounding box (818,49)–(1000,810), in drawer
text/edge, not the canvas (`city-nondeterminism-diff.png`). Instrumentation
(`determinism-probe.log`) shows active 180ms CSS transitions; drawer bounding
x was 806.863 and 891.083 on the two runs instead of final x=580. Visibility
was satisfied during the slide. rAF animation was already fixed at its first
frame, so freezing that could not solve this failure.

The recipe now waits for fixture HTTP requests to finish, requires city content
and the loaded Codex index, and finishes finite CSS animations at their normal
endpoint before capture. An additional baseline Codex repeat caught the old
recipe photographing an asynchronous loading state; the explicit index-ready
check resolves that independently. No production animation or renderer changed.

## Reproduce

From this worktree, with Python Pillow/Playwright and Firefox installed:

```sh
python docs/reviews/mobil-helskarm/browser_check.py > docs/reviews/mobil-helskarm/firefox.json
python docs/reviews/mobil-helskarm/browser_check.py --baseline > docs/reviews/mobil-helskarm/baseline-firefox.json
python docs/reviews/mobil-helskarm/compare.py > docs/reviews/mobil-helskarm/comparison.json
node --test $(rg --files web/static/js -g '*.test.mjs')
# From server/: clean environment, no live .env inherited
# env -i HOME=/home/tk PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOTEST_PORT=57900 go vet ./...
```

`SHA256SUMS` hashes the final PNG files and proof recipes/results. Images named
`firefox-390-*.png` and `firefox-1000-*.png` are fix; `baseline-` marks baseline.
Scenes: city, hex, march with city selector, notification drawer, individual
notice, account, search, Codex. View at 1:1.

## Limits / gates

Actual HTML/CSS/modules/renderers with an **explicit HTTP fixture**, frozen time,
intervals and rAF; main startup is excluded to avoid connecting to a live world.
No built server/DB provenance is claimed for these images. This is not a natural
register/join acceptance run. Account profile/placement details outside the
fixture are incomplete. DOM-dispatched backdrop/outside events test those
branches explicitly; they are not represented as physical taps on an exposed
mobile backdrop. No server/API/CLI or game semantics changed. Code and fixture
interaction checks pass; visual approval and integration belong to Claude /
Timothy. No deploy check applies.
