# Expedition return notification

Contract: Timothy2026-10-20 asks for English mission-complete wording and an
estimated local clock arrival rather than a world-day number. Reuse authoritative
arrival tick and world anchor, keep reason in stored report, and degrade visibly
when no clock anchor exists. Scope: shared time projection, web/CLI text/tests,
Codex. No mission duration, return reason or old event reinterpretation. TEXT gate
belongs in playtest; no new layout. Acceptance: wall ETA follows cadence/anchor,
no raw day-number ETA or misleading no-unseen-ground primary wording.

Targeted CLI suite and683JS GREEN. World-clock tests cover cadence changes and
missing anchors; HTTP CLI test caches one authoritative clock read and renders
23:12 for tick252 at60sec/tick from tick250 anchored23:10. Two physical controls
(wrong clock projection and old day-only wording) produce named RED. Historical
reason data is retained; only presentation changes.
