// Package chronicle records every domain event to an append-only JSONL log and
// a daily human-readable Markdown prose file, both stored under CHRONICLE_DIR.
// It hooks into events.Store as a Sink — Record is called after every successful
// Append. Failures here MUST NOT break the event write; we log and move on.
package chronicle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// localTZ is the timezone all chronicle timestamps are rendered in.
// Sweden — we never log in UTC. Falls back to time.Local if tzdata is missing.
var localTZ = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Stockholm"); err == nil {
		return loc
	}
	return time.Local
}()

// Chronicler writes one JSONL line and one Markdown paragraph per event.
type Chronicler struct {
	mu sync.Mutex

	dir       string
	pool      *pgxpool.Pool
	worldName string
	worldSlug string

	jsonlFile *os.File

	mdDate string // YYYY-MM-DD currently open
	mdFile *os.File

	settlementNames  map[uuid.UUID]string
	settlementOwners map[uuid.UUID]string // settlement_id → player username
	playerNames      map[uuid.UUID]string
}

// Open creates or opens chronicle files for the given world. The world name is
// fetched once at startup and used in filenames. If dir is empty, all writes
// become no-ops (chronicling disabled).
func Open(ctx context.Context, dir string, pool *pgxpool.Pool, worldID uuid.UUID) (*Chronicler, error) {
	if dir == "" {
		return &Chronicler{}, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir chronicle dir: %w", err)
	}

	var worldName string
	if err := pool.QueryRow(ctx, `SELECT name FROM worlds WHERE id = $1`, worldID).Scan(&worldName); err != nil {
		return nil, fmt.Errorf("load world name: %w", err)
	}
	slug := slugify(worldName)

	jsonlPath := filepath.Join(dir, worldID.String()+".jsonl")
	jf, err := os.OpenFile(jsonlPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open jsonl: %w", err)
	}

	c := &Chronicler{
		dir:              dir,
		pool:             pool,
		worldName:        worldName,
		worldSlug:        slug,
		jsonlFile:        jf,
		settlementNames:  map[uuid.UUID]string{},
		settlementOwners: map[uuid.UUID]string{},
		playerNames:      map[uuid.UUID]string{},
	}
	if err := c.openTodayMD(); err != nil {
		_ = jf.Close()
		return nil, err
	}
	slog.Info("chronicle opened", "world", worldName, "dir", dir)
	return c, nil
}

// Close flushes and closes both files.
func (c *Chronicler) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []string
	if c.jsonlFile != nil {
		if err := c.jsonlFile.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if c.mdFile != nil {
		if err := c.mdFile.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("chronicle close: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Record is called by events.Store after every successful Append. Errors here
// are logged but never returned — chronicling must not break gameplay.
func (c *Chronicler) Record(ctx context.Context, e events.SinkEvent) {
	if c == nil || c.jsonlFile == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("chronicle record panic", "err", r, "event_type", e.EventType, "event_id", e.ID)
		}
	}()

	c.mu.Lock()
	defer c.mu.Unlock()

	actor, subject := c.resolveNames(ctx, e)
	category := categorize(e.EventType)
	summary := renderSummary(e, actor, subject)

	line := struct {
		Timestamp  string          `json:"ts"`
		EventID    int64           `json:"event_id"`
		World      uuid.UUID       `json:"world"`
		Category   string          `json:"category"`
		Actor      string          `json:"actor,omitempty"`
		Subject    string          `json:"subject,omitempty"`
		Summary    string          `json:"summary"`
		StreamType string          `json:"stream_type"`
		EventType  string          `json:"event_type"`
		Detail     json.RawMessage `json:"detail"`
	}{
		Timestamp:  e.CreatedAt.In(localTZ).Format(time.RFC3339Nano),
		EventID:    e.ID,
		World:      e.WorldID,
		Category:   category,
		Actor:      actor,
		Subject:    subject,
		Summary:    summary,
		StreamType: e.StreamType,
		EventType:  e.EventType,
		Detail:     e.Payload,
	}

	raw, err := json.Marshal(line)
	if err != nil {
		slog.Error("chronicle marshal", "err", err, "event_id", e.ID)
		return
	}
	if _, err := c.jsonlFile.Write(append(raw, '\n')); err != nil {
		slog.Error("chronicle jsonl write", "err", err, "event_id", e.ID)
	}

	if err := c.ensureMDIsToday(e.CreatedAt); err != nil {
		slog.Error("chronicle md rotate", "err", err)
		return
	}
	mdLine := fmt.Sprintf("- **%s** — %s\n",
		e.CreatedAt.In(localTZ).Format("15:04:05"), summary)
	if _, err := c.mdFile.WriteString(mdLine); err != nil {
		slog.Error("chronicle md write", "err", err, "event_id", e.ID)
	}
}

// ── name resolution ─────────────────────────────────────────────────────────

func (c *Chronicler) resolveNames(ctx context.Context, e events.SinkEvent) (actor, subject string) {
	// Subject: best-effort from stream — most streams are settlements.
	switch e.StreamType {
	case "province", "combat":
		subject = c.settlementName(ctx, e.StreamID)
	case "kingdom":
		subject = c.kingdomName(ctx, e.StreamID)
	}

	// Actor: dig in payload for common keys.
	var payload map[string]any
	if err := json.Unmarshal(e.Payload, &payload); err == nil {
		for _, k := range []string{"player_id", "owner_id", "attacker_id", "sender_id", "from_player_id"} {
			if v, ok := payload[k].(string); ok && v != "" {
				if id, err := uuid.Parse(v); err == nil {
					actor = c.playerName(ctx, id)
					break
				}
			}
		}
	}

	// Fall back to settlement owner when actor is still unknown.
	if actor == "" && e.StreamType == "province" && e.StreamID != uuid.Nil {
		actor = c.settlementOwnerName(ctx, e.StreamID)
	}
	return actor, subject
}

func (c *Chronicler) settlementName(ctx context.Context, id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	if n, ok := c.settlementNames[id]; ok {
		return n
	}
	var name string
	_ = c.pool.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, id).Scan(&name)
	if name == "" {
		name = "okänd bosättning"
	}
	c.settlementNames[id] = name
	return name
}

func (c *Chronicler) playerName(ctx context.Context, id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	if n, ok := c.playerNames[id]; ok {
		return n
	}
	var name string
	_ = c.pool.QueryRow(ctx, `SELECT username FROM players WHERE id = $1`, id).Scan(&name)
	if name == "" {
		name = "okänd Wanax"
	}
	c.playerNames[id] = name
	return name
}

func (c *Chronicler) settlementOwnerName(ctx context.Context, settlementID uuid.UUID) string {
	if n, ok := c.settlementOwners[settlementID]; ok {
		return n
	}
	var name string
	_ = c.pool.QueryRow(ctx,
		`SELECT p.username FROM players p JOIN settlements s ON s.owner_id = p.id WHERE s.id = $1`,
		settlementID).Scan(&name)
	c.settlementOwners[settlementID] = name
	return name
}

func (c *Chronicler) kingdomName(ctx context.Context, id uuid.UUID) string {
	var name string
	_ = c.pool.QueryRow(ctx, `SELECT name FROM kingdoms WHERE id = $1`, id).Scan(&name)
	if name == "" {
		return "okänt kingdom"
	}
	return name
}

// ── md rotation ─────────────────────────────────────────────────────────────

func (c *Chronicler) openTodayMD() error {
	return c.ensureMDIsToday(time.Now().In(localTZ))
}

func (c *Chronicler) ensureMDIsToday(t time.Time) error {
	d := t.In(localTZ).Format("2006-01-02")
	if d == c.mdDate && c.mdFile != nil {
		return nil
	}
	if c.mdFile != nil {
		_ = c.mdFile.Close()
	}
	fname := fmt.Sprintf("megaron_%s_chronicle_%s.md", c.worldSlug, d)
	path := filepath.Join(c.dir, fname)
	stat, _ := os.Stat(path)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open md: %w", err)
	}
	if stat == nil {
		header := fmt.Sprintf("# %s — krönika %s\n\n*Genererad av Megaron chronicle. Tider i lokal tid (Europa/Stockholm).*\n\n", c.worldName, d)
		if _, err := f.WriteString(header); err != nil {
			_ = f.Close()
			return err
		}
	}
	c.mdFile = f
	c.mdDate = d
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "world"
	}
	return s
}

func categorize(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "Build"):
		return "build"
	case strings.HasPrefix(eventType, "Train"):
		return "train"
	case strings.HasPrefix(eventType, "Combat") || eventType == "Battle":
		return "combat"
	case strings.HasPrefix(eventType, "Trade"):
		return "trade"
	case strings.HasPrefix(eventType, "Kharis") || strings.HasPrefix(eventType, "Divine"):
		return "kharis"
	case strings.HasPrefix(eventType, "Kingdom"):
		return "kingdom"
	case strings.HasPrefix(eventType, "Messenger"):
		return "messenger"
	case strings.HasPrefix(eventType, "Loyalty"):
		return "loyalty"
	case strings.HasPrefix(eventType, "Starvation"):
		return "starvation"
	case strings.HasPrefix(eventType, "Gossip"):
		return "gossip"
	case strings.HasPrefix(eventType, "March"):
		return "march"
	default:
		return "other"
	}
}

// renderSummary turns an event into one short Swedish prose line.
// Unknown types get a generic rendering — never empty.
func renderSummary(e events.SinkEvent, actor, subject string) string {
	var p map[string]any
	_ = json.Unmarshal(e.Payload, &p)

	a := actor
	if a == "" {
		a = "Någon"
	}
	s := subject
	if s == "" {
		s = "okänd bosättning"
	}

	switch e.EventType {
	case "BuildComplete":
		return fmt.Sprintf("%s färdigställde bygge av %s i %s", a, str(p, "building_type"), s)
	case "TrainComplete":
		return fmt.Sprintf("%s rekryterade %v %s i %s", a, p["count"], str(p, "unit_type"), s)
	case "CombatResolved":
		switch str(p, "outcome") {
		case "attacker_victory":
			return fmt.Sprintf("Strid avgjord i %s — anfallaren segrade", s)
		case "defender_victory":
			return fmt.Sprintf("Strid avgjord i %s — försvararen segrade", s)
		default:
			return fmt.Sprintf("Strid avgjord i %s — %s", s, str(p, "outcome"))
		}
	case "TradeDelivery":
		return fmt.Sprintf("Handelsleverans till %s: %v %s", s, p["quantity"], str(p, "good_key"))
	case "TradeLost":
		return fmt.Sprintf("Handel förlorad på väg till %s: %v %s (%s)", s, p["quantity"], str(p, "good_key"), str(p, "reason"))
	case "TradeReturn":
		return fmt.Sprintf("Handelsretur till %s: %v %s levererat", s, p["quantity"], str(p, "good_key"))
	case "KharisMissedMaintenance":
		return fmt.Sprintf("Templet i %s försummades — gudarnas tålamod minskar", s)
	case "KharisMaintained":
		return fmt.Sprintf("Templet i %s underhölls vederbörligen", s)
	case "KharisLost":
		return fmt.Sprintf("Kharis sjönk i %s (%v)", s, str(p, "reason"))
	case "DivinePunishment":
		return fmt.Sprintf("Gudomligt straff i %s: %s", s, str(p, "type"))
	case "DivineBlessing":
		return fmt.Sprintf("Gudomlig välsignelse i %s: %s", s, str(p, "type"))
	case "StarvationDamage":
		return fmt.Sprintf("Hunger i %s — bosättningen svälter", s)
	case "MessengerArrived":
		return fmt.Sprintf("Budbärare anlände till %s", s)
	case "MessengerReturned":
		return fmt.Sprintf("Budbärare återvände till %s", s)
	case "LoyaltyDecay":
		return fmt.Sprintf("Lojaliteten i %s eroderar", s)
	case "LoyaltyChanged":
		return fmt.Sprintf("Lojaliteten i %s ändrades till %v", s, p["new_level"])
	// ── discrete-unit model (C1–C8): unit stream carries a unit_id, not a settlement,
	// so render from payload coords rather than the (unknown) settlement subject. ──
	case "UnitMarchOrdered":
		return fmt.Sprintf("En styrka marscherar från (%v,%v) mot (%v,%v)",
			p["origin_q"], p["origin_r"], p["target_q"], p["target_r"])
	case "UnitArrived":
		return fmt.Sprintf("En styrka anlände till (%v,%v)", p["q"], p["r"])
	case "UnitDeserted":
		return fmt.Sprintf("Soldater deserterade (%v man) — silverbrist", p["lost"])
	case "UnitAttrition":
		return fmt.Sprintf("Soldater gick förlorade (%v man) — spannmålsbrist", p["lost"])
	case "UnitFormed":
		return "En ny styrka börjar formeras"
	case "UnitReinforced":
		return "En styrka förstärktes"
	case "UnitDeployed":
		return "En styrka ställdes upp i garnison"
	case "UnitDisbanded":
		return "En styrka upplöstes — männen återvände till befolkningen"
	case "UnitStanceChanged":
		return fmt.Sprintf("En styrka ändrade hållning till %s", str(p, "stance"))
	case "UnitCombatResolved", "UnitIntercepted":
		return fmt.Sprintf("Strid avgjordes för en styrka vid (%v,%v)", p["q"], p["r"])
	case "CityCollapsed":
		return fmt.Sprintf("%s kollapsade (%s) — en warband reser sig ur ruinerna", s, str(p, "cause"))
	case "OutpostEstablished":
		return fmt.Sprintf("%s upprättade en utpost", a)
	default:
		return fmt.Sprintf("[%s] %s (%s)", e.EventType, s, compactPayload(e.Payload))
	}
}

func str(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func compactPayload(raw json.RawMessage) string {
	s := string(raw)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
