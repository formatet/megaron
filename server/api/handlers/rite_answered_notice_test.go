package handlers

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

type riteNoticeRecorder struct {
	kinds  []string
	bodies []map[string]any
}

func (r *riteNoticeRecorder) BroadcastEvent(uuid.UUID, string, any) {}
func (r *riteNoticeRecorder) NotifyPlayer(_ context.Context, _, _ uuid.UUID, kind string, _ int, body any) error {
	r.kinds = append(r.kinds, kind)
	m, _ := body.(map[string]any)
	r.bodies = append(r.bodies, m)
	return nil
}

// The one who prayed gets a notice with the answer: god, prayer, outcome
// and the find's position (byggplan A.2). Their own act, so it is direct.
func TestRiteAnswered_NoticeNamesTheGodAndTheFind(t *testing.T) {
	f := newAlphaFixture(t)
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO map_tiles(world_id,q,r,terrain,copper_deposit) VALUES($1,9,0,'hills',true)`, f.world); err != nil {
		t.Fatal(err)
	}
	hub := &riteNoticeRecorder{}
	f.h.Hub = hub
	if r := f.cast(f.cities[0], religion.PrayerPotniaVision); r.Code != 200 {
		t.Fatalf("cast %d %s", r.Code, r.Body.String())
	}
	if len(hub.kinds) != 1 || hub.kinds[0] != "RiteAnswered" {
		t.Fatalf("notices %v, want one RiteAnswered", hub.kinds)
	}
	b := hub.bodies[0]
	if b["god_id"] != "potnia" || b["prayer"] != religion.PrayerPotniaVision || b["success"] != true {
		t.Fatalf("body %v: want potnia, the prayer, success", b)
	}
	if b["prayer_name"] != "Potnia's Vision" || b["message"] == "" || b["name"] != "Alpha City" {
		t.Fatalf("body %v: want prayer name, the answer and the city", b)
	}
	if b["q"] != 9 || b["r"] != 0 {
		t.Fatalf("find at q,r = %v,%v, want 9,0", b["q"], b["r"])
	}
}
