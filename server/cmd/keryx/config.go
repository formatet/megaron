package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Server     string `json:"server"`
	Token      string `json:"token"`
	WorldID    string `json:"world_id"`
	ProvinceID string `json:"province_id"`
	PlayerID   string `json:"player_id"`
	Username   string `json:"username"`
}

func configPath() string {
	if p := os.Getenv("POLEIA_CONFIG"); p != "" {
		return p
	}
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "poleia", "config.json")
}

func loadConfig() (*Config, error) {
	// Env vars take full precedence over stored config.
	if server := os.Getenv("POLEIA_SERVER"); server != "" {
		return &Config{
			Server: server,
			Token:  os.Getenv("POLEIA_TOKEN"),
		}, nil
	}

	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Env var token overrides stored token.
	if t := os.Getenv("POLEIA_TOKEN"); t != "" {
		cfg.Token = t
	}
	return &cfg, nil
}

// saveConfig merges the known fields into whatever the file already holds,
// instead of marshalling Config over the top of it.
//
// Config models only the six fields the CLI itself needs, but the file is
// written by other tools too: the playtest fleet's register-agents.sh stores
// password, email, refresh_token, culture and host_unit_id in the same JSON,
// and refresh-tokens.sh needs password+email as its login fallback when a
// refresh_token has expired. A plain Marshal of Config silently DELETED every
// one of those the first time the CLI saved — which it does routinely, e.g.
// when founding sets province_id. The account then had no way back once its
// access token expired: no refresh_token to refresh with, no credentials to
// log in with. That is what killed the diomedes account (2026-07-25: LOGIN
// FAILED, "invalid credentials" — the password had been erased from under it),
// and it was days away from taking the whole fleet.
//
// Round-tripping through a map keeps unknown keys untouched, so the file stays
// a shared document rather than this struct's exclusive property.
func saveConfig(cfg *Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	merged := map[string]any{}
	if existing, err := os.ReadFile(path); err == nil {
		// A corrupt or unreadable file is not worth failing the save over —
		// fall through with an empty map and write our fields cleanly.
		_ = json.Unmarshal(existing, &merged)
	}

	ours, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(ours, &fields); err != nil {
		return err
	}
	for k, v := range fields {
		merged[k] = v
	}

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
