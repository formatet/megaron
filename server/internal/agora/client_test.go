package agora

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/account/whoami") {
			_ = json.NewEncoder(w).Encode(map[string]string{"user_id": "@temenos:matrix.test"})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	c, err := NewClient(Config{URL: server.URL, AccessToken: "admin-canary-secret", AdminRoom: "!admin:matrix.test"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func fixtureReply(requestID, sender, body string) map[string]any {
	return map[string]any{"event_id": "$reply", "sender": sender, "type": "m.room.message", "content": map[string]any{
		"msgtype": "m.notice", "body": body, "m.relates_to": map[string]any{"m.in_reply_to": map[string]string{"event_id": requestID}},
	}}
}

func TestCreateConfirmsOriginalCorrelatedReplyAfterRetry(t *testing.T) {
	var requests, creates int
	cache := map[string]string{}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-canary-secret" {
			t.Error("missing admin credential")
		}
		switch {
		case strings.Contains(r.URL.Path, "/send/"):
			requests++
			if r.Method != http.MethodPut {
				t.Error("non-idempotent send")
			}
			var content map[string]string
			_ = json.NewDecoder(r.Body).Decode(&content)
			if _, exists := cache[r.URL.Path]; !exists {
				cache[r.URL.Path] = "$original"
				creates++
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"event_id": cache[r.URL.Path]})
		case strings.Contains(r.URL.Path, "/context/"):
			// Forged and unrelated replies must not affect ownership.
			_ = json.NewEncoder(w).Encode(map[string]any{"events_after": []any{
				fixtureReply("$original", "@intruder:matrix.test", "Created user @agamemnon:matrix.test with password `bad`"),
				fixtureReply("$unrelated", "@conduit:matrix.test", "Username is not available."),
			}})
		case strings.HasSuffix(r.URL.Path, "/messages"):
			if r.URL.Query().Get("from") == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"end": "older-page", "chunk": []any{map[string]string{"event_id": "$unrelated"}}})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"chunk": []any{
					fixtureReply("$original", "@conduit:matrix.test", "| log table |\nCreated user @agamemnon:matrix.test with password `never-return-this-secret`"),
				}})
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	for i := 0; i < 2; i++ {
		password, err := NewPassword()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = c.Create(ctx, "stable-player-claim-create", "agamemnon", password)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
	if requests != 2 || creates != 1 {
		t.Fatalf("requests=%d creates=%d", requests, creates)
	}
}

func TestCreateOccupiedReplyNeverConfirmsOwnership(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/send/") {
			_ = json.NewEncoder(w).Encode(map[string]string{"event_id": "$request"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"events_after": []any{
			fixtureReply("$request", "@conduit:matrix.test", "Command failed with error:\n```\nUsername is not available.\n```"),
		}})
	})
	password, _ := NewPassword()
	if err := c.Create(context.Background(), "stable", "agamemnon", password); !errors.Is(err, ErrOccupied) {
		t.Fatalf("wanted occupied, got %v", err)
	}
}

func TestClientRedactsRemoteErrorsAndRejectsRedirect(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://admin-canary-secret.invalid")
				w.WriteHeader(status)
				_, _ = w.Write([]byte("password-canary-secret admin-canary-secret"))
			})
			password, _ := NewPassword()
			err := c.Create(context.Background(), "stable", "agamemnon", password)
			if err != ErrUnavailable || strings.Contains(err.Error(), "canary") {
				t.Fatal("remote response leaked or redirect followed")
			}
		})
	}
}

func TestUnrelatedReplyCannotConfirmAccount(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/send/"):
			_ = json.NewEncoder(w).Encode(map[string]string{"event_id": "$request"})
		case strings.Contains(r.URL.Path, "/context/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"events_after": []any{fixtureReply("$other", "@conduit:matrix.test", "Created user @agamemnon:matrix.test with password `secret`")}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"chunk": []any{map[string]string{"event_id": "$request"}}})
		}
	})
	password, _ := NewPassword()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := c.Create(ctx, "stable", "agamemnon", password); !errors.Is(err, ErrUnconfirmed) {
		t.Fatalf("unrelated response accepted: %v", err)
	}
}

func TestDisplayNameUsesEphemeralUserSessionAndLogsOut(t *testing.T) {
	var profile, logout bool
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/login"):
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "ephemeral-user-secret"})
		case strings.Contains(r.URL.Path, "/profile/"):
			if r.Header.Get("Authorization") != "Bearer ephemeral-user-secret" {
				t.Error("profile used wrong identity")
			}
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			profile = body["displayname"] == "Ágamemnon"
			_, _ = w.Write([]byte("{}"))
		case strings.HasSuffix(r.URL.Path, "/logout"):
			logout = r.Header.Get("Authorization") == "Bearer ephemeral-user-secret"
			_, _ = w.Write([]byte("{}"))
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	})
	password, _ := NewPassword()
	if err := c.SetDisplayName(context.Background(), "agamemnon", password, "Ágamemnon"); err != nil {
		t.Fatal(err)
	}
	if !profile || !logout {
		t.Fatal("profile or ephemeral logout missing")
	}
}
