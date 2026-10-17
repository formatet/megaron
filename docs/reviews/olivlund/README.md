# Olive grove — BILD contract

Problem: Timothy mistakes olive groves for maquis during live play.
Player truth: groves read as low, broad mature trees with visible trunks, distinct from low scrub at game scale.
Invariant: deterministic whole-pixel existing renderer/palette/light; no terrain, FOW, geography or mechanics changes.
Scope: only olive sprites in production map.js; renderer showcase fixtures and reproducible Firefox proof.
Acceptance: same before/after scenes at zoom1.0, minimum and close game zoom; olive/maquis adjacency, cedar, coast, cities/units/FOW; repeated images identical; no-olive control unchanged.
Stop: BILD requires Timothy's judgment before merge. Claude leads review/integration; Codex hands over branch/images/hash.
Proof: Firefox1:1 primary, grayscale browser capture as support, containment/determinism, JS plus required fresh Go/vet.
Gate: blocks legible recognition of the wood source needed for building and the chain.
These are explicitly synthetic renderer showcases, not claims about native gameplay or backend provenance.


Implementation: source commit `f8152cf9`, baseline `2bf95ac7`. Large/mid/small olive sprites grow from 7×7/5×6/5×4 to 13×10/9×7/7×6, with broad split crowns and forked trunks. Existing four foliage tones and separate trunk material are preserved. No palette, placement, terrain data or simulation change.

Primary Firefox 1:1 comparison: [before](olive-maquis-before.png) and [after](olive-maquis-after.png). Supporting scenes: [minimum zoom](olive-maquis-min-after.png), [close zoom](olive-maquis-close-after.png), [grayscale](olive-maquis-gray-after.png), [cedar boundary](cedar-after.png), [forest](forest-after.png), [coast/plains](plains-after.png). All files are original browser screenshots, DPR1, 900×780 canvas pixels; no image resizing or painting. Fixtures are deliberately synthetic and use the actual map renderer. They do not establish gameplay/backend correctness.

Reproduction: `python3 docs/reviews/olivlund/proof.py`. Firefox146.0.1; baseline renderer is loaded directly from the immutable git revision. Identical fixture, camera and frozen frame for both source arms. `proof.json` records source hashes, tile positions and pixel counts. Every scene repeats with zero changed pixels; no-olive control has zero before/after changed pixels. These assertions establish repeatability and containment, not human recognition.

Physical mutation: `python3 docs/reviews/olivlund/mutation.py` restores the old renderer, requires the named `no visible change` RED, restores exact candidate bytes, then requires the entire scene matrix GREEN. See `mutation.log`, `mutation-red.log`, `mutation-restored.log`.

Validation: 642 JavaScript tests passed, zero skips; `go vet ./...` clean. Fresh full Go suite passed on migration181 and source commit f8152cf9 (world151.524s; go.log).

STOPP: this is a visual proposal awaiting Timothy's BILD judgment via Claude. No merge, push or deploy by Codex. Pixel counts and tests do not approve the artwork.
