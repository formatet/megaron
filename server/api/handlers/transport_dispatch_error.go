package handlers

import (
	"errors"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/transport"
	"net/http"
)

func writeTransportDispatchError(w http.ResponseWriter, err error, fallback string) {
	var shortage *economy.ProvisionShortage
	if errors.As(err, &shortage) {
		writeError(w, http.StatusUnprocessableEntity, shortage.Error())
		return
	}
	var refusal *transport.DispatchRefusal
	if errors.As(err, &refusal) {
		writeError(w, http.StatusUnprocessableEntity, refusal.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, fallback)
}
