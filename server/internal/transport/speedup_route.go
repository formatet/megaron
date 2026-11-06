package transport

import "formatet/megaron/server/internal/movement"

func movementSpeedUp(r movement.StoredRoute, nowMilli int64) (movement.StoredRoute, bool) {
	return movement.SpeedUpRemaining(r, nowMilli, 3, 4)
}
