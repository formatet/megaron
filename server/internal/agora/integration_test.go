package agora

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// This opt-in test only accepts a loopback fixture and never prints credentials
// or admin-room bodies. The provisioned fixture names belong to this test.
func TestContinuwuityIsolatedProtocol(t *testing.T) {
	file := os.Getenv("AGORA_TEST_FIXTURE_FILE")
	if file == "" {
		t.Skip("isolated Continuwuity fixture not configured")
	}
	encoded, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("could not read isolated fixture")
	}
	var fixture struct {
		BaseURL string `json:"base_url"`
		Token   string `json:"temenos_token"`
		Room    string `json:"room"`
	}
	if json.Unmarshal(encoded, &fixture) != nil {
		t.Fatal("invalid isolated fixture")
	}
	address, err := url.Parse(fixture.BaseURL)
	if err != nil || address.Hostname() != "127.0.0.1" {
		t.Fatal("fixture must use IPv4 loopback")
	}
	c, err := NewClient(Config{URL: fixture.BaseURL, AccessToken: fixture.Token, AdminRoom: fixture.Room}, nil)
	if err != nil {
		t.Fatal("could not configure isolated client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	localpart := "-go-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	createTxn := "create-" + localpart
	first, _ := NewPassword()
	second, _ := NewPassword()
	if err := c.Create(ctx, createTxn, localpart, first); err != nil {
		t.Fatal("initial create failed", err)
	}
	if err := c.Create(ctx, createTxn, localpart, second); err != nil {
		t.Fatal("idempotent historic create confirmation failed", err)
	}
	if err := c.Create(ctx, "collision-"+localpart, localpart, second); err != ErrOccupied {
		t.Fatal("occupied account was not rejected")
	}
	if err := c.ResetPassword(ctx, "reset-"+localpart, localpart, second); err != nil {
		t.Fatal("reset failed", err)
	}
	login := func(password string) error {
		var response struct {
			AccessToken string `json:"access_token"`
		}
		body := map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": c.UserID(localpart)}, "password": password}
		err := c.call(ctx, http.MethodPost, "/login", "", body, &response)
		if err != nil {
			return err
		}
		return c.call(ctx, http.MethodPost, "/logout", response.AccessToken, map[string]any{}, nil)
	}
	if login(first) == nil {
		t.Fatal("old password still authenticates")
	}
	if login(second) != nil {
		t.Fatal("new password does not authenticate")
	}
	if err := c.SetDisplayName(ctx, localpart, second, "Ágamemnon fixture"); err != nil {
		t.Fatal("displayname update failed", err)
	}
	var profile struct {
		DisplayName string `json:"displayname"`
	}
	if c.call(ctx, http.MethodGet, "/profile/"+url.PathEscape(c.UserID(localpart))+"/displayname", fixture.Token, nil, &profile) != nil || profile.DisplayName != "Ágamemnon fixture" {
		t.Fatal("displayname not preserved")
	}
	if err := c.command(ctx, "deactivate-"+localpart, "!admin users deactivate -- "+c.UserID(localpart), "User "+c.UserID(localpart)+" has been deactivated", false); err != nil {
		t.Fatal("fixture deactivation failed", err)
	}
	if err := c.Create(ctx, "deactivated-collision-"+localpart, localpart, second); err != ErrOccupied {
		t.Fatal("deactivated account name reused")
	}
	// Original create ownership still remains provable after deactivation, but
	// reset/profile will fail; the reconciler must never mark such a claim ready.
}
