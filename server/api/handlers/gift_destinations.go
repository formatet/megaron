package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TransferDestinations exposes the same foreign destinations the letter gate
// permits, plus own cities. Origin uses a province id; destinations use city ids.
func (h *ProvinceHandler) TransferDestinations(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return
	}
	origin, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, 400, "invalid province ID")
		return
	}
	player, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var own bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM settlements WHERE world_id=$1 AND province_id=$2 AND owner_id=$3 AND state='active')`, worldID, origin, player).Scan(&own); err != nil {
		writeError(w, 500, "could not check origin")
		return
	}
	if !own {
		writeError(w, 403, "not your settlement")
		return
	}
	origins := loadVisibleOrigins(r.Context(), h.pool, worldID, player)
	rows, err := h.pool.Query(r.Context(), `SELECT s.id,s.name,s.owner_id,COALESCE(NULLIF(pl.wanax_name,''),pl.username),p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id JOIN players pl ON pl.id=s.owner_id WHERE s.world_id=$1 AND s.state='active' AND s.province_id<>$2 ORDER BY s.name,s.id`, worldID, origin)
	if err != nil {
		writeError(w, 500, "could not load transfer destinations")
		return
	}
	defer rows.Close()
	type destination struct {
		ID        uuid.UUID `json:"settlement_id"`
		Name      string    `json:"name"`
		Owner     uuid.UUID `json:"owner_id"`
		OwnerName string    `json:"owner_name"`
		Own       bool      `json:"own"`
	}
	list := []destination{}
	for rows.Next() {
		var d destination
		var q, r int
		if err := rows.Scan(&d.ID, &d.Name, &d.Owner, &d.OwnerName, &q, &r); err != nil {
			writeError(w, 500, "could not read transfer destination")
			return
		}
		d.Own = d.Owner == player
		if d.Own || province.VisibleFrom(province.MapPosition{Q: q, R: r}, origins, 6) {
			list = append(list, d)
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "could not read transfer destinations")
		return
	}
	writeJSON(w, 200, list)
}

// GiftHistory returns only this Wanax's own dispatches and notified outcomes.
// Intended recipients cannot read dispatches before the physical cargo arrives.
func (h *ProvinceHandler) GiftHistory(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return
	}
	player, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT event_type,payload,created_at FROM (SELECT DISTINCT ON(payload->>'transport_id') event_type,payload,created_at,id FROM events WHERE world_id=$1 AND ((event_type='GiftDispatched' AND payload->>'sender_id'=$2) OR (event_type IN('GiftDelivered','GiftLost') AND (payload->>'sender_id'=$2 OR payload->>'recipient_id'=$2 OR payload->>'actual_recipient_id'=$2))) ORDER BY payload->>'transport_id',id DESC) gifts ORDER BY id DESC LIMIT 200`, worldID, player.String())
	if err != nil {
		writeError(w, 500, "could not load gifts")
		return
	}
	defer rows.Close()
	type gift struct {
		Kind      string          `json:"kind"`
		Body      json.RawMessage `json:"body"`
		CreatedAt time.Time       `json:"created_at"`
	}
	list := []gift{}
	for rows.Next() {
		var g gift
		if err := rows.Scan(&g.Kind, &g.Body, &g.CreatedAt); err != nil {
			writeError(w, 500, "could not read gift")
			return
		}
		list = append(list, g)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not read gifts")
		return
	}
	writeJSON(w, 200, list)
}
