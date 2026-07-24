package main

import (
	"encoding/json"
	"os"
	"testing"
)

// A CLI save must not delete fields other tools store in the same file.
func TestSaveConfigPreservesForeignFields(t *testing.T) {
	path := t.TempDir() + "/config.json"
	t.Setenv("POLEIA_CONFIG", path)

	seed := `{"server":"http://x","token":"gammal","world_id":"w1","player_id":"p1",
	          "username":"Test","password":"hemlig","email":"t@x.local",
	          "refresh_token":"rt","culture":"khemetiu"}`
	if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Token = "ny"
	cfg.ProvinceID = "prov-1" // what founding writes
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	for k, want := range map[string]string{
		"password": "hemlig", "email": "t@x.local",
		"refresh_token": "rt", "culture": "khemetiu",
	} {
		if got[k] != want {
			t.Errorf("%s = %v, want %q — a CLI save erased a field it does not own", k, got[k], want)
		}
	}
	if got["token"] != "ny" || got["province_id"] != "prov-1" {
		t.Errorf("own fields not written: token=%v province_id=%v", got["token"], got["province_id"])
	}
}
