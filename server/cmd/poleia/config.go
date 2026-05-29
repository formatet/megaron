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

func saveConfig(cfg *Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
