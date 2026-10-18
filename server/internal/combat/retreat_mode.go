package combat

import "net/http"

const RetreatPercentNotAllowed = "retreat_at_loss is no longer a player choice; choose hold_to_last_man: true or when_losing: true"

// ValidateRetreatMode is shared by direct setters and dispatch prechecks. A
// retreat order replaces the mode; it never partly edits a fixed threshold.
func ValidateRetreatMode(retreatAtLoss *float64, hold, whenLosing bool) *OrderReject {
	if retreatAtLoss != nil {
		return reject(http.StatusBadRequest, RetreatPercentNotAllowed)
	}
	if hold == whenLosing {
		return reject(http.StatusBadRequest, "choose exactly one of hold_to_last_man: true or when_losing: true")
	}
	return nil
}
