package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWsDialTarget(t *testing.T) {
	cases := []struct {
		name     string
		server   string
		token    string
		worldID  string
		wantURL  string
		wantAuth string
		wantErr  bool
	}{
		{
			name:     "http becomes ws",
			server:   "http://10.0.1.88:8080",
			token:    "tok-123",
			worldID:  "world-1",
			wantURL:  "ws://10.0.1.88:8080/ws/world-1",
			wantAuth: "Bearer tok-123",
		},
		{
			name:     "https becomes wss, path prefix preserved",
			server:   "https://megaron.example.com/api",
			token:    "tok-456",
			worldID:  "world-2",
			wantURL:  "wss://megaron.example.com/api/ws/world-2",
			wantAuth: "Bearer tok-456",
		},
		{
			name:     "no token means no Authorization header",
			server:   "http://localhost:8080",
			token:    "",
			worldID:  "world-3",
			wantURL:  "ws://localhost:8080/ws/world-3",
			wantAuth: "",
		},
		{
			name:    "unsupported scheme is an error",
			server:  "ftp://example.com",
			worldID: "world-4",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, header, err := wsDialTarget(tc.server, tc.token, tc.worldID)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("wsDialTarget(%q): want error, got none (url=%q)", tc.server, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("wsDialTarget(%q): unexpected error: %v", tc.server, err)
			}
			if got != tc.wantURL {
				t.Errorf("wsDialTarget(%q) url = %q, want %q", tc.server, got, tc.wantURL)
			}
			gotAuth := header.Get("Authorization")
			if gotAuth != tc.wantAuth {
				t.Errorf("wsDialTarget(%q) Authorization = %q, want %q", tc.server, gotAuth, tc.wantAuth)
			}
		})
	}
}

// echoWSServer starts a real httptest server that performs a genuine
// WebSocket upgrade (gorilla) and writes the given messages to the client,
// one at a time, then blocks — proving runWatch against an actual handshake,
// not a mock. Returns the server (caller must Close it).
func echoWSServer(t *testing.T, msgs [][]byte) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for _, m := range msgs {
			if err := conn.WriteMessage(websocket.TextMessage, m); err != nil {
				return
			}
		}
		// Keep the connection open a bit so a slow reader isn't cut short by
		// an immediate server-side close before it has read everything.
		time.Sleep(200 * time.Millisecond)
	}))
	return srv
}

// echoWSServerClosing is like echoWSServer but closes the connection right
// after writing the last message instead of holding it open — used where the
// test wants runWatch's read loop to run to a natural connection-closed end
// rather than stopping early via --count.
func echoWSServerClosing(t *testing.T, msgs [][]byte) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for _, m := range msgs {
			if err := conn.WriteMessage(websocket.TextMessage, m); err != nil {
				return
			}
		}
	}))
	return srv
}

func dialTestServer(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	return conn
}

// TestRunWatch_SkipsHeartbeats sends 3 heartbeats interleaved with 2 real
// events, then the server closes the connection — so with an unbounded
// count (0), runWatch reads every frame until the close error and the ONLY
// way shown can land on 2 is if heartbeats never incremented the counter.
// A version that treats Heartbeat as an ordinary message would report 5.
func TestRunWatch_SkipsHeartbeats(t *testing.T) {
	heartbeat, _ := json.Marshal(wsMsg{Kind: "Heartbeat"})
	real1, _ := json.Marshal(wsMsg{Kind: "ForeignMarchSighted", WorldID: "w1", Payload: json.RawMessage(`{"owner":"Nestor"}`)})
	real2, _ := json.Marshal(wsMsg{Kind: "BattleWon", WorldID: "w1", Payload: json.RawMessage(`{"outcome":"attacker_wins"}`)})

	srv := echoWSServerClosing(t, [][]byte{heartbeat, real1, heartbeat, real2, heartbeat})
	defer srv.Close()
	conn := dialTestServer(t, srv)
	defer conn.Close()

	// &Client{tickSecondsFetched: true} is the degrade-path stub (rad K):
	// "fetched" but no cadence recorded, so any game-day rendering these
	// tests happen to trigger falls back to the wall-clock countdown rather
	// than making a real HTTP call.
	shown, _ := runWatch(&Client{tickSecondsFetched: true}, conn, watchOptions{count: 0}) // unbounded: runs until the server closes
	if shown != 2 {
		t.Fatalf("runWatch shown = %d, want 2 (heartbeats must never count)", shown)
	}
}

// TestRunWatch_CountStopsAtN proves --count's early exit: with 3 real
// messages available and count=1, runWatch must return after the first one
// with a nil error (a clean stop, not a connection failure) — this is what
// backs `keryx watch --count 1` exiting 0 after a single event.
func TestRunWatch_CountStopsAtN(t *testing.T) {
	m1, _ := json.Marshal(wsMsg{Kind: "ForeignMarchSighted", Payload: json.RawMessage(`{"owner":"Nestor"}`)})
	m2, _ := json.Marshal(wsMsg{Kind: "BattleWon", Payload: json.RawMessage(`{}`)})
	m3, _ := json.Marshal(wsMsg{Kind: "BattleLost", Payload: json.RawMessage(`{}`)})

	srv := echoWSServer(t, [][]byte{m1, m2, m3})
	defer srv.Close()
	conn := dialTestServer(t, srv)
	defer conn.Close()

	shown, err := runWatch(&Client{tickSecondsFetched: true}, conn, watchOptions{count: 1})
	if err != nil {
		t.Fatalf("runWatch: unexpected error: %v", err)
	}
	if shown != 1 {
		t.Fatalf("runWatch shown = %d, want 1", shown)
	}
}

func TestRunWatch_KindFilter(t *testing.T) {
	m1, _ := json.Marshal(wsMsg{Kind: "SitosFundLow", Payload: json.RawMessage(`{}`)})
	m2, _ := json.Marshal(wsMsg{Kind: "ForeignMarchSighted", Payload: json.RawMessage(`{"owner":"Nestor"}`)})

	srv := echoWSServer(t, [][]byte{m1, m2})
	defer srv.Close()
	conn := dialTestServer(t, srv)
	defer conn.Close()

	shown, err := runWatch(&Client{tickSecondsFetched: true}, conn, watchOptions{kinds: map[string]bool{"ForeignMarchSighted": true}, count: 1})
	if err != nil {
		t.Fatalf("runWatch: unexpected error: %v", err)
	}
	if shown != 1 {
		t.Fatalf("runWatch shown = %d, want 1 (filtered kind must not count)", shown)
	}
}

func TestWatchKindFilter(t *testing.T) {
	if got := watchKindFilter(""); got != nil {
		t.Errorf("watchKindFilter(\"\") = %v, want nil", got)
	}
	set := watchKindFilter("A, B ,C")
	for _, k := range []string{"A", "B", "C"} {
		if !set[k] {
			t.Errorf("watchKindFilter: expected %q in set %v", k, set)
		}
	}
	if len(set) != 3 {
		t.Errorf("watchKindFilter: got %d entries, want 3: %v", len(set), set)
	}
}
