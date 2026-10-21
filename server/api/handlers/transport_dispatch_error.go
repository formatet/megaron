package handlers

import (
	"errors"
	"formatet/megaron/server/internal/economy"
	"net/http"
)

func writeTransportDispatchError(w http.ResponseWriter, err error, fallback string) {
	var shortage *economy.ProvisionShortage
	if errors.As(err, &shortage) {
		writeError(w, http.StatusUnprocessableEntity, shortage.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, fallback)
}
