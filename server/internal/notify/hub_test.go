package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// dialHub starts an httptest server that upgrades every request and hands the
// connection to the hub under playerID, then returns a connected client. The
// returned closer shuts the whole thing down. Pass uuid.Nil for playerID to
// simulate an unauthenticated connection.
func dialHub(t *testing.T, h *Hub, worldID, playerID uuid.UUID) (*websocket.Conn, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgraderForTest.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.Register(conn, worldID, playerID) // blocks until closed
	}))
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	return c, func() { c.Close(); srv.Close() }
}

var upgraderForTest = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// clientCount reads the hub's client-map size under the lock.
func clientCount(h *Hub) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// TestBroadcastDelivers confirms a broadcast reaches a connected client.
func TestBroadcastDelivers(t *testing.T) {
	h := New()
	world := uuid.New()
	c, closer := dialHub(t, h, world, uuid.Nil)
	defer closer()

	if !waitFor(t, time.Second, func() bool { return clientCount(h) == 1 }) {
		t.Fatalf("client never registered")
	}

	h.Broadcast(world, Msg{Kind: "ArmyArrival"})
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(string(raw), `"ArmyArrival"`) {
			break // ignore any heartbeat that may race in
		}
	}
}

// TestDisconnectDeregisters confirms a client that drops is removed from the hub
// (both pumps tear down and the map drains).
func TestDisconnectDeregisters(t *testing.T) {
	h := New()
	world := uuid.New()
	c, closer := dialHub(t, h, world, uuid.Nil)
	defer closer()

	if !waitFor(t, time.Second, func() bool { return clientCount(h) == 1 }) {
		t.Fatalf("client never registered")
	}
	c.Close()
	if !waitFor(t, 2*time.Second, func() bool { return clientCount(h) == 0 }) {
		t.Fatalf("client never deregistered after close")
	}
}

// TestBroadcastDuringChurn hammers Broadcast while clients connect and drop
// concurrently. It guards the teardown ordering: a broadcast must never send on
// a closed channel (that would panic and crash the process). Run with -race.
func TestBroadcastDuringChurn(t *testing.T) {
	h := New()
	world := uuid.New()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Broadcaster: fire continuously from several goroutines.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					h.Broadcast(world, Msg{Kind: "Churn"})
				}
			}
		}()
	}

	// Churn: connect and abruptly drop clients while broadcasts fly.
	for i := 0; i < 60; i++ {
		c, closer := dialHub(t, h, world, uuid.Nil)
		time.Sleep(2 * time.Millisecond)
		c.Close()
		closer()
	}

	close(stop)
	wg.Wait()

	if !waitFor(t, 3*time.Second, func() bool { return clientCount(h) == 0 }) {
		t.Fatalf("clients leaked after churn: %d remain", clientCount(h))
	}
}

// readKind reads frames off c until one whose "kind" field matches want, or
// the deadline passes and the test fails. Heartbeats (which carry their own
// "kind":"Heartbeat") are transparently skipped so they can't be mistaken for
// the awaited message or mask its absence.
func readKind(t *testing.T, c *websocket.Conn, want string, d time.Duration) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(d))
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: waiting for %q: %v", want, err)
		}
		if strings.Contains(string(raw), `"kind":"`+want+`"`) {
			return
		}
	}
}

// expectNoMessage asserts c receives nothing but heartbeats before the
// deadline passes — used to prove a client that should NOT be a NotifyPlayer
// target really gets nothing.
func expectNoMessage(t *testing.T, c *websocket.Conn, d time.Duration) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(d))
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			return // deadline hit (or conn tore down) — nothing arrived, as expected
		}
		if strings.Contains(string(raw), `"kind":"Heartbeat"`) {
			continue
		}
		t.Fatalf("unexpected message: %s", raw)
	}
}

// TestNotifyPlayerTargetsOnlyThatPlayer is the regression guard for the FOW
// leak fixed 2026-07-25: NotifyPlayer used to ignore the playerID it was
// given for delivery and broadcast to the whole world regardless (only the DB
// persistence honoured it). Two clients share a world with different
// playerIDs; a NotifyPlayer aimed at A must reach ONLY A.
func TestNotifyPlayerTargetsOnlyThatPlayer(t *testing.T) {
	h := New()
	world := uuid.New()
	playerA := uuid.New()
	playerB := uuid.New()

	connA, closerA := dialHub(t, h, world, playerA)
	defer closerA()
	connB, closerB := dialHub(t, h, world, playerB)
	defer closerB()

	if !waitFor(t, time.Second, func() bool { return clientCount(h) == 2 }) {
		t.Fatalf("clients never registered")
	}

	if err := h.NotifyPlayer(context.Background(), world, playerA, "OfferAccepted", 1, nil); err != nil {
		t.Fatalf("NotifyPlayer: %v", err)
	}

	readKind(t, connA, "OfferAccepted", 2*time.Second)
	expectNoMessage(t, connB, 300*time.Millisecond)
}

// TestBroadcastEventReachesEveryPlayer confirms genuinely world-wide events
// still reach every client regardless of playerID — the fix must not turn
// BroadcastEvent into a targeted send too, only NotifyPlayer's delivery path.
func TestBroadcastEventReachesEveryPlayer(t *testing.T) {
	h := New()
	world := uuid.New()

	connA, closerA := dialHub(t, h, world, uuid.New())
	defer closerA()
	connB, closerB := dialHub(t, h, world, uuid.New())
	defer closerB()

	if !waitFor(t, time.Second, func() bool { return clientCount(h) == 2 }) {
		t.Fatalf("clients never registered")
	}

	h.BroadcastEvent(world, "SeasonTurnover", nil)

	readKind(t, connA, "SeasonTurnover", 2*time.Second)
	readKind(t, connB, "SeasonTurnover", 2*time.Second)
}

// TestNotifyPlayerNilBroadcasts confirms the documented uuid.Nil escape
// hatch: callers with no specific recipient (playerID == uuid.Nil) still
// reach every client in the world, preserving the pre-fix contract that
// several existing callers deliberately rely on.
func TestNotifyPlayerNilBroadcasts(t *testing.T) {
	h := New()
	world := uuid.New()

	connA, closerA := dialHub(t, h, world, uuid.New())
	defer closerA()
	connB, closerB := dialHub(t, h, world, uuid.Nil)
	defer closerB()

	if !waitFor(t, time.Second, func() bool { return clientCount(h) == 2 }) {
		t.Fatalf("clients never registered")
	}

	if err := h.NotifyPlayer(context.Background(), world, uuid.Nil, "WorldAnnouncement", 1, nil); err != nil {
		t.Fatalf("NotifyPlayer: %v", err)
	}

	readKind(t, connA, "WorldAnnouncement", 2*time.Second)
	readKind(t, connB, "WorldAnnouncement", 2*time.Second)
}
