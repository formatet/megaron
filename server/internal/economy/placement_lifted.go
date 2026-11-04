package economy

import (
	"context"
	"fmt"
	"strings"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

// LiftedPlacement is one settlement_placement row removed by
// PrunePlacementsToPopulation: which gubbe left which work.
type LiftedPlacement struct {
	Ordinal      int
	TargetKind   string // "hex" | "building"
	HexQ, HexR   *int
	BuildingType string
	BuildingID   *uuid.UUID
	GodID        *string
	GoodKey      string
}

// EventPlacementsLiftedV1 / PlacementsLifted: a population drop took citizens
// off their work. Event type on the province stream; same name as the notification.
const (
	EventPlacementsLiftedV1  = "PlacementsLiftedV1"
	NotifPlacementsLifted    = "PlacementsLifted"
	notifPlacementsLiftedLvl = 4 // same level as BuildComplete: "something in the city changed"
)

// Causes of a lift.
const (
	LiftCauseDraft         = "draft"
	LiftCauseReinforcement = "reinforcement"
	LiftCauseDecline       = "decline"
)

// PlacementsLiftedPayload is the outcome (event payload and notification body).
type PlacementsLiftedPayload struct {
	SettlementID uuid.UUID          `json:"settlement_id"`
	City         string             `json:"city"`
	Cause        string             `json:"cause"`
	Lifted       []LiftedPlacementJ `json:"lifted"`
	Text         string             `json:"text"`
}

// LiftedPlacementJ is the wire form of a LiftedPlacement.
type LiftedPlacementJ struct {
	Ordinal      int    `json:"ordinal"`
	Kind         string `json:"kind"`
	HexQ         *int   `json:"hex_q,omitempty"`
	HexR         *int   `json:"hex_r,omitempty"`
	Good         string `json:"good"`
	BuildingType string `json:"building_type,omitempty"`
	God          string `json:"god,omitempty"`
}

func liftCauseText(cause string) string {
	switch cause {
	case LiftCauseDraft:
		return "men were drafted"
	case LiftCauseReinforcement:
		return "men were sent to reinforce a unit"
	default:
		return "the city has fewer people than posts"
	}
}

// PlacementsLiftedText is the one English sentence every surface shows.
// templeName turns a god id into "Temple of X" (economy may not import religion).
func PlacementsLiftedText(city, cause string, lifted []LiftedPlacement, templeName func(string) string) string {
	counts := map[string]int{}
	long := map[string]string{} // short form -> form after "1 "
	var order []string
	for _, l := range lifted {
		var short, one string
		switch {
		case l.TargetKind == "hex" && l.HexQ != nil && l.HexR != nil:
			short = fmt.Sprintf("%s at (%d, %d)", l.GoodKey, *l.HexQ, *l.HexR)
			one = "on " + short
		case l.GoodKey == GoodCult:
			short = "serving at the temple"
			if l.GodID != nil {
				t := templeName(*l.GodID)
				short = "serving at the " + strings.ToLower(t[:1]) + t[1:]
			}
			one = short
		default:
			short = fmt.Sprintf("%s (%s)", strings.ToLower(province.BuildingDisplayName(l.BuildingType)), l.GoodKey)
			one = "at the " + short
		}
		if counts[short] == 0 {
			order = append(order, short)
			long[short] = one
		}
		counts[short]++
	}
	n := len(lifted)
	var b strings.Builder
	if n == 1 {
		fmt.Fprintf(&b, "A citizen left their work in %s: 1 %s", city, long[order[0]])
	} else {
		fmt.Fprintf(&b, "%d citizens left their work in %s: ", n, city)
		for i, w := range order {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s ×%d", w, counts[w])
		}
	}
	fmt.Fprintf(&b, " (%s).", liftCauseText(cause))
	return b.String()
}

// LiftedToWire converts lifted rows to their JSON form (notification body, draft response).
func LiftedToWire(lifted []LiftedPlacement) []LiftedPlacementJ {
	out := make([]LiftedPlacementJ, 0, len(lifted))
	for _, l := range lifted {
		j := LiftedPlacementJ{Ordinal: l.Ordinal, Kind: l.TargetKind, HexQ: l.HexQ, HexR: l.HexR, Good: l.GoodKey, BuildingType: l.BuildingType}
		if l.GodID != nil {
			j.God = *l.GodID
		}
		out = append(out, j)
	}
	return out
}

// AnnouncePlacementsLifted records PlacementsLiftedV1 and tells the owner. Call it
// AFTER the transaction that lifted has committed. No-op when nothing was lifted.
// hub and store may be nil (tests).
func AnnouncePlacementsLifted(ctx context.Context, q Tx, store *events.Store, hub BlockadeNotifier, worldID, settlementID uuid.UUID, cause string, lifted []LiftedPlacement, templeName func(string) string) {
	if len(lifted) == 0 {
		return
	}
	var ownerID uuid.UUID
	var city string
	if err := q.QueryRow(ctx, `SELECT COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'::uuid), name FROM settlements WHERE id = $1`, settlementID).Scan(&ownerID, &city); err != nil {
		return
	}
	p := PlacementsLiftedPayload{SettlementID: settlementID, City: city, Cause: cause,
		Text: PlacementsLiftedText(city, cause, lifted, templeName)}
	p.Lifted = LiftedToWire(lifted)
	if store != nil {
		_, _ = store.Append(ctx, settlementID, events.StreamProvince, EventPlacementsLiftedV1, p, worldID, nil)
	}
	if hub != nil && ownerID != uuid.Nil {
		_ = hub.NotifyPlayer(ctx, worldID, ownerID, NotifPlacementsLifted, notifPlacementsLiftedLvl, p)
	}
}
