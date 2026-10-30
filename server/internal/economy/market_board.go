package economy

import (
	"context"
	"errors"
	"fmt"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The market board (megaron_plan_marknadstavlan, Timothy 2026-10-28): a ruler
// with a Market writes what the city buys and sells. Other rulers' journeys
// read it on site, within the market's reach, and their readings age as
// memory. Nothing here looks at stock or rates — the board may lie or be empty.

// BoardMaxGoods is how many goods each of the Buys and Sells lines may name.
const BoardMaxGoods = 3

// BoardReach is how far from the city hex a passer-by reads the board, by
// market level: level 1 reaches the catchment, each level beyond reaches further.
func BoardReach(level int) int {
	switch {
	case level <= 0:
		return -1
	case level == 1:
		return 2
	case level == 2:
		return 4
	default:
		return 6
	}
}

type boardDB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// MarketBoard is a city's board as it stands now. WrittenTick is nil while the
// owner has never written it: the market exists, the board is empty.
type MarketBoard struct {
	SettlementID uuid.UUID
	OwnerID      uuid.UUID
	Pos          province.MapPosition
	Level        int
	Buys, Sells  []string
	WrittenTick  *int
}

const boardSelect = `SELECT s.id, s.owner_id, p.map_q, p.map_r, MAX(b.level),
        COALESCE(mb.buys, '{}'), COALESCE(mb.sells, '{}'), mb.written_tick
   FROM settlements s
   JOIN provinces p ON p.id = s.province_id
   JOIN buildings b ON b.settlement_id = s.id AND b.building_type = 'market' AND b.level >= 1
   LEFT JOIN market_boards mb ON mb.settlement_id = s.id
  WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state = 'active'`

const boardGroup = ` GROUP BY s.id, s.owner_id, p.map_q, p.map_r, mb.buys, mb.sells, mb.written_tick`

func scanBoards(rows pgx.Rows) ([]MarketBoard, error) {
	defer rows.Close()
	var out []MarketBoard
	for rows.Next() {
		var b MarketBoard
		if err := rows.Scan(&b.SettlementID, &b.OwnerID, &b.Pos.Q, &b.Pos.R, &b.Level, &b.Buys, &b.Sells, &b.WrittenTick); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// LoadMarketBoards returns every readable board in the world: each active,
// owned city with a finished Market, written or not.
func LoadMarketBoards(ctx context.Context, db boardDB, worldID uuid.UUID) ([]MarketBoard, error) {
	rows, err := db.Query(ctx, boardSelect+boardGroup, worldID)
	if err != nil {
		return nil, err
	}
	return scanBoards(rows)
}

// LoadMarketBoard returns one city's board, or nil when it has no Market.
func LoadMarketBoard(ctx context.Context, db boardDB, worldID, settlementID uuid.UUID) (*MarketBoard, error) {
	rows, err := db.Query(ctx, boardSelect+` AND s.id = $2`+boardGroup, worldID, settlementID)
	if err != nil {
		return nil, err
	}
	boards, err := scanBoards(rows)
	if err != nil || len(boards) == 0 {
		return nil, err
	}
	return &boards[0], nil
}

// RecordBoardReading stores what reader saw on the board at tick. A newer
// reading replaces an older one; replaying the same reading changes nothing.
func RecordBoardReading(ctx context.Context, db boardDB, worldID, readerID uuid.UUID, b MarketBoard, tick int) error {
	_, err := db.Exec(ctx,
		`INSERT INTO market_board_readings (world_id, player_id, settlement_id, buys, sells, written_tick, read_tick)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (player_id, settlement_id) DO UPDATE
		    SET buys = EXCLUDED.buys, sells = EXCLUDED.sells,
		        written_tick = EXCLUDED.written_tick, read_tick = EXCLUDED.read_tick
		  WHERE market_board_readings.read_tick <= EXCLUDED.read_tick`,
		worldID, readerID, b.SettlementID, b.Buys, b.Sells, b.WrittenTick, tick)
	return err
}

// ReadBoardsAt lets reader's journey, standing at pos at tick, read every other
// ruler's board within reach. A board read from beyond the city hex also makes
// the city's hex known by word (player_heard_tiles): reader can write there,
// but heard hexes never open trade offers (Timothy 2026-10-28 20:53).
func ReadBoardsAt(ctx context.Context, db contactDB, worldID, readerID uuid.UUID, boards []MarketBoard, pos province.MapPosition, tick int) error {
	for _, b := range boards {
		if b.OwnerID == readerID || province.HexDistance(b.Pos, pos) > BoardReach(b.Level) {
			continue
		}
		if err := RecordBoardReading(ctx, db, worldID, readerID, b, tick); err != nil {
			return err
		}
		if err := RecordContact(ctx, db, worldID, readerID, b.SettlementID, ContactMarketBoard, tick); err != nil {
			return err
		}
		if _, err := db.Exec(ctx,
			`INSERT INTO player_heard_tiles (world_id, player_id, q, r, source, heard_tick)
			 VALUES ($1, $2, $3, $4, 'board', $5) ON CONFLICT DO NOTHING`,
			worldID, readerID, b.Pos.Q, b.Pos.R, tick); err != nil {
			return err
		}
	}
	return nil
}

// BoardReading is what a player last read on another city's board.
type BoardReading struct {
	Buys, Sells []string
	WrittenTick *int
	ReadTick    int
}

// LoadBoardReadings returns playerID's readings keyed by settlement.
func LoadBoardReadings(ctx context.Context, db boardDB, worldID, playerID uuid.UUID) (map[uuid.UUID]BoardReading, error) {
	rows, err := db.Query(ctx,
		`SELECT settlement_id, buys, sells, written_tick, read_tick
		   FROM market_board_readings WHERE world_id = $1 AND player_id = $2`, worldID, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]BoardReading{}
	for rows.Next() {
		var id uuid.UUID
		var r BoardReading
		if err := rows.Scan(&id, &r.Buys, &r.Sells, &r.WrittenTick, &r.ReadTick); err != nil {
			return nil, err
		}
		out[id] = r
	}
	return out, rows.Err()
}

// ValidateBoard checks a board the owner wants to write. allowed is the trade
// catalogue (the goods a trade offer may name). The error text is player-facing.
func ValidateBoard(buys, sells []string, allowed map[string]bool) error {
	if len(buys) > BoardMaxGoods || len(sells) > BoardMaxGoods {
		return fmt.Errorf("a market board names at most %d goods to buy and %d to sell", BoardMaxGoods, BoardMaxGoods)
	}
	seen := map[string]string{}
	for _, l := range []struct {
		line  string
		goods []string
	}{{"buys", buys}, {"sells", sells}} {
		line := l.line
		for _, g := range l.goods {
			if !allowed[g] {
				return fmt.Errorf("%q cannot be traded, so it cannot go on the board", g)
			}
			if prev, ok := seen[g]; ok {
				if prev == line {
					return fmt.Errorf("%s is on the board twice", g)
				}
				return fmt.Errorf("the board cannot both buy and sell %s", g)
			}
			seen[g] = line
		}
	}
	return nil
}

// ErrNoMarket is returned by WriteMarketBoard when the city has no finished Market.
var ErrNoMarket = errors.New("the city needs a Market before it can put up a board")

// WriteMarketBoard replaces settlementID's board. Empty lines clear it; the
// row stays so readers see "the board is empty" as the owner's own choice.
func WriteMarketBoard(ctx context.Context, db boardDB, worldID, settlementID uuid.UUID, buys, sells []string, tick int) error {
	b, err := LoadMarketBoard(ctx, db, worldID, settlementID)
	if err != nil {
		return err
	}
	if b == nil {
		return ErrNoMarket
	}
	if buys == nil {
		buys = []string{}
	}
	if sells == nil {
		sells = []string{}
	}
	_, err = db.Exec(ctx,
		`INSERT INTO market_boards (settlement_id, world_id, buys, sells, written_tick)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (settlement_id) DO UPDATE
		    SET buys = EXCLUDED.buys, sells = EXCLUDED.sells, written_tick = EXCLUDED.written_tick`,
		settlementID, worldID, buys, sells, tick)
	return err
}
