package agora

import (
	"errors"
	"net/url"
	"strings"
)

// Config is deliberately independent of the process environment.
type Config struct {
	URL         string
	AccessToken string
	AdminRoom   string
}

// ConfigFromEnv enables integration only when all three values are present.
func ConfigFromEnv(getenv func(string) string) (Config, bool, error) {
	cfg := Config{
		URL:         strings.TrimSpace(getenv("POLEIA_AGORA_URL")),
		AccessToken: strings.TrimSpace(getenv("POLEIA_AGORA_ACCESS_TOKEN")),
		AdminRoom:   strings.TrimSpace(getenv("POLEIA_AGORA_ADMIN_ROOM")),
	}
	if cfg.URL == "" || cfg.AccessToken == "" || cfg.AdminRoom == "" {
		return Config{}, false, nil
	}
	if err := cfg.validate(); err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

func (c Config) validate() error {
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return errors.New("invalid Agora homeserver URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return errors.New("Agora requires HTTPS or loopback HTTP")
	}
	if c.AccessToken == "" || !strings.HasPrefix(c.AdminRoom, "!") {
		return errors.New("invalid Agora credentials configuration")
	}
	return nil
}
