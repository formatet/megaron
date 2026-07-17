package notify

import (
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
// connection to the hub, then returns a connected client. The returned closer
// shuts the whole thing down.
func dialHub(t *testing.T, h *Hub, worldID uuid.UUID) (*websocket.Conn, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgraderForTest.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.Register(conn, worldID) // blocks until closed
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
	c, closer := dialHub(t, h, world)
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
	c, closer := dialHub(t, h, world)
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
		c, closer := dialHub(t, h, world)
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
