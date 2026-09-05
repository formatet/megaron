package province

import (
	"context"

	"github.com/google/uuid"
)

// ResolveTradeRoute decides whether a goods route between two settlements
// sails (naval) or marches (land), and returns the hex distance to use for
// travel-time on whichever category is picked.
//
// Call only once both endpoints are already known coastal-or-harboured (the
// caller's job — the same "coastal OR has a harbour" gate
// api/handlers/unit.go's embark/disembark checks use; this function does not
// repeat it, since a harbour lives in the buildings table, which this
// package never touches).
//
// naval is picked only when a navigable sea route actually connects the two
// settlements: each endpoint's real departure point is its nearest adjacent
// sea/river hex (NearestSeaNeighbor — a settlement's own hex is never itself
// sea, so FindPath can never start there), and FindPath must find a
// naval-passable route between those two points. Any failure (no adjacent
// water, or land blocks every sea lane between them) falls back to land,
// with the UNCHANGED straight-line HexDistance a land route has always
// used — an existing land route must never cost anything different than it
// did before naval selection existed.
//
// dist for naval is the resolved sea path's own hex count (len(path)-1,
// mirroring how combat.StartMarch already resolves a ship's real departure/
// arrival hex and prices its ETA off the real path rather than a straight
// line) — a coastline is not a straight line, so reusing the land distance
// for a sea leg would silently mis-time (and, for callers who price per hex,
// mis-price) every voyage.
func ResolveTradeRoute(ctx context.Context, db Queryer, worldID uuid.UUID, originCoastal, destCoastal bool, origin, dest MapPosition) (category string, dist int, err error) {
	landDist := HexDistance(origin, dest)
	if !originCoastal || !destCoastal {
		return "land", landDist, nil
	}

	oq, oR, foundOrigin, err := NearestSeaNeighbor(ctx, db, worldID, origin.Q, origin.R)
	if err != nil {
		return "", 0, err
	}
	dq, dR, foundDest, err := NearestSeaNeighbor(ctx, db, worldID, dest.Q, dest.R)
	if err != nil {
		return "", 0, err
	}
	if !foundOrigin || !foundDest {
		return "land", landDist, nil
	}

	path, _, ok, err := FindPath(ctx, db, worldID,
		MapPosition{Q: oq, R: oR}, MapPosition{Q: dq, R: dR}, "naval")
	if err != nil {
		return "", 0, err
	}
	if !ok {
		return "land", landDist, nil
	}
	return "naval", len(path) - 1, nil
}
