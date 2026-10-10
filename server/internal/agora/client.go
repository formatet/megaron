package agora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnavailable = errors.New("Agora is unavailable; try again later")
	ErrOccupied    = errors.New("Agora username is occupied")
	ErrUnconfirmed = errors.New("Agora command has no confirmed response")
)

// Client uses the verified Continuwuity 26.9.1 admin-room protocol.
// A successful correlated CREATE reply, never account existence, proves ownership.
type Client struct {
	cfg        Config
	http       *http.Client
	serverName string
	botID      string
	identityMu sync.Mutex
}

func NewClient(cfg Config, transport *http.Client) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if transport == nil {
		transport = &http.Client{Timeout: 20 * time.Second}
	}
	// Redirects must not forward either admin or newly created user credentials.
	copyHTTP := *transport
	copyHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{cfg: cfg, http: &copyHTTP}, nil
}

func (c *Client) Homeserver() string { return strings.TrimRight(c.cfg.URL, "/") }
func (c *Client) UserID(localpart string) string {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	return "@" + localpart + ":" + c.serverName
}

// Initialize resolves identity from the access token, since modern Matrix room
// IDs are opaque and do not necessarily contain a homeserver name.
func (c *Client) Initialize(ctx context.Context) error {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	if c.serverName != "" {
		return nil
	}
	var identity struct {
		UserID string `json:"user_id"`
	}
	if err := c.call(ctx, http.MethodGet, "/account/whoami", c.cfg.AccessToken, nil, &identity); err != nil {
		return err
	}
	parts := strings.SplitN(identity.UserID, ":", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "@") || parts[1] == "" {
		return ErrUnavailable
	}
	c.serverName = parts[1]
	c.botID = "@conduit:" + parts[1]
	return nil
}

func (c *Client) call(ctx context.Context, method, path, token string, body any, result any) error {
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return ErrUnavailable
		}
		requestBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Homeserver()+"/_matrix/client/v3"+path, requestBody)
	if err != nil {
		return ErrUnavailable
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrUnavailable
	}
	if result == nil {
		return nil
	}
	// Never expose remote error bodies or parse errors: they may contain secrets.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(result); err != nil {
		return ErrUnavailable
	}
	return nil
}

type roomEvent struct {
	EventID string `json:"event_id"`
	Sender  string `json:"sender"`
	Type    string `json:"type"`
	Content struct {
		MsgType  string `json:"msgtype"`
		Body     string `json:"body"`
		Relation struct {
			Reply struct {
				EventID string `json:"event_id"`
			} `json:"m.in_reply_to"`
		} `json:"m.relates_to"`
	} `json:"content"`
}

// command reuses txnID across crashes. Matrix returns the original request event.
// Historical reply lookup lets recovery confirm that exact original operation.
func (c *Client) command(ctx context.Context, txnID, command string, successPrefix string, occupied bool) error {
	if err := c.Initialize(ctx); err != nil {
		return err
	}
	var sent struct {
		EventID string `json:"event_id"`
	}
	path := "/rooms/" + url.PathEscape(c.cfg.AdminRoom) + "/send/m.room.message/" + url.PathEscape(txnID)
	if err := c.call(ctx, http.MethodPut, path, c.cfg.AccessToken, map[string]string{"msgtype": "m.text", "body": command}, &sent); err != nil {
		return err
	}
	if sent.EventID == "" {
		return ErrUnconfirmed
	}
	// Fetch context first for the common case. Poll until the caller's deadline.
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		matched, err := c.findReply(ctx, sent.EventID, successPrefix, occupied)
		if matched || err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ErrUnconfirmed
		case <-ticker.C:
		}
	}
}

func (c *Client) replyOutcome(event roomEvent, requestID, prefix string, occupied bool) (bool, error) {
	if event.Sender != c.botID || event.Type != "m.room.message" || event.Content.MsgType != "m.notice" || event.Content.Relation.Reply.EventID != requestID {
		return false, nil
	}
	// Log-table prefixes precede the final response in this version. Inspect lines
	// rather than requiring a raw-body prefix, and never return the raw content.
	for _, line := range strings.Split(event.Content.Body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true, nil
		}
	}
	if occupied && strings.Contains(event.Content.Body, "Username is not available.") {
		return true, ErrOccupied
	}
	return true, ErrUnavailable
}

func (c *Client) findReply(ctx context.Context, requestID, prefix string, occupied bool) (bool, error) {
	base := "/rooms/" + url.PathEscape(c.cfg.AdminRoom)
	var nearby struct {
		After []roomEvent `json:"events_after"`
	}
	if err := c.call(ctx, http.MethodGet, base+"/context/"+url.PathEscape(requestID)+"?limit=100", c.cfg.AccessToken, nil, &nearby); err != nil {
		return false, err
	}
	for _, event := range nearby.After {
		if matched, err := c.replyOutcome(event, requestID, prefix, occupied); matched {
			return true, err
		}
	}
	// A command may be old after a restart; context is bounded. Walk backward
	// to its request, stopping safely rather than adopting an unrelated account.
	from := ""
	for page := 0; page < 100; page++ {
		var history struct {
			End   string      `json:"end"`
			Chunk []roomEvent `json:"chunk"`
		}
		query := "?dir=b&limit=100"
		if from != "" {
			query += "&from=" + url.QueryEscape(from)
		}
		if err := c.call(ctx, http.MethodGet, base+"/messages"+query, c.cfg.AccessToken, nil, &history); err != nil {
			return false, err
		}
		for _, event := range history.Chunk {
			if matched, err := c.replyOutcome(event, requestID, prefix, occupied); matched {
				return true, err
			}
			if event.EventID == requestID {
				return false, nil
			}
		}
		if history.End == "" || history.End == from || len(history.Chunk) == 0 {
			return false, nil
		}
		from = history.End
	}
	return false, ErrUnconfirmed
}

func validLocalpart(localpart string) bool {
	normal, err := Localpart(localpart)
	return err == nil && normal == localpart
}

// Create confirms the original successful operation when replaying txnID.
func (c *Client) Create(ctx context.Context, txnID, localpart, password string) error {
	if !validLocalpart(localpart) || !safePassword(password) {
		return ErrUnavailable
	}
	if err := c.Initialize(ctx); err != nil {
		return err
	}
	return c.command(ctx, txnID, "!admin users create-user -- "+localpart+" "+password, "Created user "+c.UserID(localpart)+" with password ", true)
}

func safePassword(password string) bool {
	if len(password) < 32 {
		return false
	}
	for _, r := range password {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func (c *Client) ResetPassword(ctx context.Context, txnID, localpart, password string) error {
	if !validLocalpart(localpart) || !safePassword(password) {
		return ErrUnavailable
	}
	if err := c.Initialize(ctx); err != nil {
		return err
	}
	return c.command(ctx, txnID, "!admin users reset-password -- "+localpart+" "+password, "Successfully reset the password for user "+c.UserID(localpart)+": ", false)
}

// Deactivate closes an erased player's Agora account. Success text measured on
// 26.9.1 (tools/agora_acceptance/README.md). Replaying txnID confirms the
// original operation, so a crash between command and bookkeeping is safe.
func (c *Client) Deactivate(ctx context.Context, txnID, localpart string) error {
	if !validLocalpart(localpart) {
		return ErrUnavailable
	}
	if err := c.Initialize(ctx); err != nil {
		return err
	}
	return c.command(ctx, txnID, "!admin users deactivate -- "+c.UserID(localpart), "User "+c.UserID(localpart)+" has been deactivated", false)
}

// SetDisplayName authenticates with an ephemeral provisioning password. Neither
// password nor user token is persisted, and the temporary session is logged out.
func (c *Client) SetDisplayName(ctx context.Context, localpart, password, name string) error {
	if err := c.Initialize(ctx); err != nil {
		return err
	}
	var login struct {
		AccessToken string `json:"access_token"`
	}
	body := map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": c.UserID(localpart)}, "password": password, "initial_device_display_name": "Megaron account provisioning"}
	if err := c.call(ctx, http.MethodPost, "/login", "", body, &login); err != nil {
		return err
	}
	if login.AccessToken == "" {
		return ErrUnavailable
	}
	err := c.call(ctx, http.MethodPut, "/profile/"+url.PathEscape(c.UserID(localpart))+"/displayname", login.AccessToken, map[string]string{"displayname": name}, nil)
	logoutErr := c.call(ctx, http.MethodPost, "/logout", login.AccessToken, map[string]any{}, nil)
	if err != nil {
		return err
	}
	return logoutErr
}
