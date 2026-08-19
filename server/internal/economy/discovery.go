package economy

// Discovery-signal tunables for the wants/exports market intel surface
// (api/handlers/province.go MarketWants). PR1 (system-computed local price)
// was repealed 2026-08-19 — [[megaron_valtabell]] PR1 — because Megaron has no
// clearing mechanism, so a price had nothing to clear against and was purely
// advisory. The discovery signal itself is not cosmetic (it's the anti-"no
// visible seller" tool), so it survives, rerooted from price onto the same
// stock+rate a settlement's own economy already tracks.
const (
	// WantsDaysCover: a settlement "wants" a good when its stock covers fewer
	// than this many days at its current drain rate. Kept low relative to
	// ExportsDaysCover — shortage should read as urgent, not just "below
	// average".
	WantsDaysCover = 5.0
	// ExportsDaysCover: a settlement "exports" a good once its built-up stock
	// covers more than this many days at its current production rate — a
	// deliberately generous buffer so a settlement only advertises a good it
	// can spare, not one it might need next tick.
	ExportsDaysCover = 20.0
	// MinFlowForCover floors |rate| so days-of-cover stays finite when a good's
	// net rate is at or near zero (division by ~0 would otherwise blow up the
	// cover calculation for a stalled or idle good).
	MinFlowForCover = 1.0
)
