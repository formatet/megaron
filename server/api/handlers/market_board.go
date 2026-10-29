package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MarketBoard handles PUT /worlds/:worldID/settlements/:settlementID/market-board.
// The owner writes what the city buys and sells (megaron_plan_marknadstavlan).
// The board is the ruler's own word: nothing is checked against stock.
func (h *SettlementHandler) MarketBoard(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	settlementID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		Buys  []string `json:"buys"`
		Sells []string `json:"sells"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not write the board")
		return
	}
	defer tx.Rollback(ctx)

	var ownerID *uuid.UUID
	var name string
	var tick int
	if err := tx.QueryRow(ctx,
		`SELECT s.owner_id, s.name, w.current_tick FROM settlements s JOIN worlds w ON w.id = s.world_id
		  WHERE s.id = $1 AND s.world_id = $2 FOR UPDATE OF s`, settlementID, worldID,
	).Scan(&ownerID, &name, &tick); err != nil {
		writeError(w, http.StatusNotFound, "settlement not found")
		return
	}
	if ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}
	goods, err := tradeableGoodsCatalog(ctx, h.pool)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load goods catalogue")
		return
	}
	allowed := map[string]bool{}
	for _, g := range goods {
		allowed[g.Key] = true
	}
	if err := economy.ValidateBoard(req.Buys, req.Sells, allowed); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := economy.WriteMarketBoard(ctx, tx, worldID, settlementID, req.Buys, req.Sells, tick); err != nil {
		if errors.Is(err, economy.ErrNoMarket) {
			writeError(w, http.StatusUnprocessableEntity, name+" has no Market. Build one before putting up a board.")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not write the board")
		return
	}
	board := map[string]any{"buys": nonNil(req.Buys), "sells": nonNil(req.Sells), "written_tick": tick}
	if _, err := h.eventStore.AppendTx(ctx, tx, settlementID, events.StreamProvince, "MarketBoardWritten", board, worldID, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write the board")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write the board")
		return
	}
	writeJSON(w, http.StatusOK, board)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// marketBoardView is the board as the viewer knows it: the owner sees their
// own board as it stands (nil without a Market); anyone else sees their last
// reading (nil if they never read it).
func marketBoardView(ctx context.Context, pool *pgxpool.Pool, worldID, settlementID, viewer uuid.UUID, own bool) map[string]any {
	if own {
		b, err := economy.LoadMarketBoard(ctx, pool, worldID, settlementID)
		if err != nil || b == nil {
			return nil
		}
		return map[string]any{"buys": nonNil(b.Buys), "sells": nonNil(b.Sells), "level": b.Level,
			"reach": economy.BoardReach(b.Level), "max_goods": economy.BoardMaxGoods, "empty": b.WrittenTick == nil}
	}
	readings, err := economy.LoadBoardReadings(ctx, pool, worldID, viewer)
	if err != nil {
		return nil
	}
	reading, ok := readings[settlementID]
	if !ok {
		return nil
	}
	anchor, err := tick.LoadAnchor(ctx, pool, worldID)
	if err != nil {
		return nil
	}
	return boardReadingJSON(reading, anchor)
}

// boardReadingJSON: when it was read is wall clock; how old it is, game days.
func boardReadingJSON(r economy.BoardReading, anchor tick.Anchor) map[string]any {
	age := anchor.Tick - r.ReadTick
	if age < 0 {
		age = 0
	}
	return map[string]any{"buys": nonNil(r.Buys), "sells": nonNil(r.Sells),
		"read_at": anchor.WallAt(int64(r.ReadTick) * 1000), "read_game_days_ago": age,
		"empty": len(r.Buys) == 0 && len(r.Sells) == 0}
}

// loadHeardTiles returns the hexes playerID knows only by word
// (player_heard_tiles). Deliberately not part of loadRememberedTiles: those
// feed gifts, founding and trade gates, and a heard hex opens none of them.
func loadHeardTiles(ctx context.Context, db interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, worldID, playerID uuid.UUID) map[[2]int]bool {
	out := map[[2]int]bool{}
	rows, err := db.Query(ctx, `SELECT q, r FROM player_heard_tiles WHERE world_id = $1 AND player_id = $2`, worldID, playerID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var q, r int
		if rows.Scan(&q, &r) == nil {
			out[[2]int{q, r}] = true
		}
	}
	return out
}
