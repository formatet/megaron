package agora

import "testing"

func TestIncompleteConfigDisablesIntegration(t *testing.T) {
	values := map[string]string{
		"POLEIA_AGORA_URL":          "https://matrix.test",
		"POLEIA_AGORA_ACCESS_TOKEN": "secret",
		"POLEIA_AGORA_ADMIN_ROOM":   "!admin:matrix.test",
	}
	for missing := range values {
		t.Run(missing, func(t *testing.T) {
			cfg, enabled, err := ConfigFromEnv(func(key string) string {
				if key == missing {
					return ""
				}
				return values[key]
			})
			if err != nil || enabled || cfg != (Config{}) {
				t.Fatal("incomplete configuration did not disable integration")
			}
		})
	}
}

func TestConfigRejectsUnsafeURLsWithoutEchoingCredentials(t *testing.T) {
	for _, address := range []string{
		"http://matrix.test", "https://secret@matrix.test", "https://matrix.test?secret=x",
		"https://matrix.test/path", "https://matrix.test#secret", "::::",
	} {
		cfg := Config{URL: address, AccessToken: "secret", AdminRoom: "!admin:matrix.test"}
		if cfg.validate() == nil {
			t.Fatalf("unsafe URL accepted: %s", address)
		}
	}
	for _, address := range []string{"https://matrix.test", "http://127.0.0.1:18099", "http://[::1]:18099"} {
		if (Config{URL: address, AccessToken: "secret", AdminRoom: "!admin:matrix.test"}).validate() != nil {
			t.Fatalf("safe URL rejected: %s", address)
		}
	}
}
