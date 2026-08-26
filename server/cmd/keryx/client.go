package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	server       string
	token        string
	http         *http.Client
	cfg          *Config // for province self-heal after a respawn relocates the capital
	extraHeaders map[string]string

	// tickSeconds/tickSecondsFetched cache the world's runtime tick cadence
	// (rad K, megaron_plan_cli_sanning.md) — fetched at most once per Client,
	// i.e. once per CLI invocation, since a fresh Client is created inside
	// every command's RunE (see newClient). Deliberately NOT persisted to
	// disk/config: a world's tick cadence can change between deploys, and a
	// stale cached value would silently mis-render every game-day ETA for
	// the rest of that world's life. One extra GET per process is the price
	// of always being current.
	tickSeconds        float64
	tickSecondsFetched bool
}

func newClient(cfg *Config) *Client {
	return &Client{
		server: cfg.Server,
		token:  cfg.Token,
		http:   &http.Client{Timeout: 15 * time.Second},
		cfg:    cfg,
	}
}

func (c *Client) do(method, path string, body any) ([]byte, int, error) {
	return c.doWithHeal(method, path, body, true)
}

func (c *Client) doWithHeal(method, path string, body any, allowHeal bool) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.server+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range c.extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, resp.StatusCode, err
	}

	// Self-heal a stale province_id: after a respawn the player's capital moves to a
	// new province, but the local config still points at the lost one, so every
	// province-scoped call returns 403 "not your province". Re-resolve the current
	// capital once, persist it, and retry the original request against the new ID.
	if allowHeal && resp.StatusCode == http.StatusForbidden && c.cfg != nil &&
		c.cfg.ProvinceID != "" && strings.Contains(path, c.cfg.ProvinceID) &&
		isNotYourProvince(data) {
		oldID := c.cfg.ProvinceID
		if newID := c.resolveCapital(); newID != "" && newID != oldID {
			c.cfg.ProvinceID = newID
			_ = saveConfig(c.cfg)
			return c.doWithHeal(method, strings.ReplaceAll(path, oldID, newID), body, false)
		}
	}
	return data, resp.StatusCode, err
}

// isNotYourProvince reports whether an error body is the ownership-check rejection.
func isNotYourProvince(data []byte) bool {
	var e struct {
		Error string `json:"error"`
	}
	return json.Unmarshal(data, &e) == nil && e.Error == "not your province"
}

// resolveCapital fetches the player's province markers and returns the province ID
// of their current capital, or "" if none is found. Used to recover from respawn.
func (c *Client) resolveCapital() string {
	data, status, err := c.doWithHeal("GET",
		fmt.Sprintf("/api/v1/worlds/%s/provinces", c.cfg.WorldID), nil, false)
	if err != nil || status >= 400 {
		return ""
	}
	var markers []struct {
		ID        string `json:"id"`
		IsCapital bool   `json:"is_capital"`
	}
	if json.Unmarshal(data, &markers) != nil {
		return ""
	}
	for _, m := range markers {
		if m.IsCapital {
			return m.ID
		}
	}
	return ""
}

// TickSeconds returns the world's runtime tick cadence in real seconds — one
// tick is one game-day ("Ticket ÄR dygnet", megaron_plan_ticket_ar_dygnet),
// so this is what lets an arrival timestamp be rendered as game-days instead
// of a wall-clock countdown (rad K, megaron_plan_cli_sanning.md). Reads the
// existing GET /worlds/:worldID (api/handlers/world.go), which already
// returns tick_seconds — no new server surface needed. The second return is
// false when the cadence couldn't be established (no world configured, the
// request failed, or the field was missing/zero), so callers degrade to a
// wall-clock-only display rather than showing a wrong or zero game-day
// count.
func (c *Client) TickSeconds() (float64, bool) {
	if c.tickSecondsFetched {
		return c.tickSeconds, c.tickSeconds > 0
	}
	c.tickSecondsFetched = true
	if c.cfg == nil || c.cfg.WorldID == "" {
		return 0, false
	}
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s", c.cfg.WorldID))
	if err != nil {
		return 0, false
	}
	var w struct {
		TickSeconds float64 `json:"tick_seconds"`
	}
	if json.Unmarshal(data, &w) != nil || w.TickSeconds <= 0 {
		return 0, false
	}
	c.tickSeconds = w.TickSeconds
	return c.tickSeconds, true
}

func (c *Client) get(path string) ([]byte, error) {
	data, status, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(data, status)
	}
	return data, nil
}

func (c *Client) post(path string, body any) ([]byte, error) {
	data, status, err := c.do("POST", path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(data, status)
	}
	return data, nil
}

func (c *Client) put(path string, body any) ([]byte, error) {
	data, status, err := c.do("PUT", path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(data, status)
	}
	return data, nil
}

func (c *Client) patch(path string, body any) ([]byte, error) {
	data, status, err := c.do("PATCH", path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(data, status)
	}
	return data, nil
}

func (c *Client) delete(path string) ([]byte, error) {
	data, status, err := c.do("DELETE", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(data, status)
	}
	return data, nil
}

func apiError(body []byte, status int) error {
	var e struct {
		Error   string `json:"error"`
		Missing []struct {
			Good string  `json:"good"`
			Need float64 `json:"need"`
			Have float64 `json:"have"`
		} `json:"missing"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Error != "" {
		if len(e.Missing) > 0 {
			parts := make([]string, len(e.Missing))
			for i, m := range e.Missing {
				parts[i] = fmt.Sprintf("%s (need %.0f, have %.0f)", m.Good, m.Need, m.Have)
			}
			return fmt.Errorf("%s: %s (HTTP %d)", e.Error, strings.Join(parts, ", "), status)
		}
		return fmt.Errorf("%s (HTTP %d)", e.Error, status)
	}
	return fmt.Errorf("HTTP %d", status)
}
