package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
)

// GPT14–15: `unit patrol --q 57 --r 26` answered "target hex not found" for a
// sea hex the map listed. The march handler reads target_q/target_r; patrol and
// post sent q/r, so every order aimed at (0,0).
func TestPatrolAndPostSendTargetHex(t *testing.T) {
	const unitID = "dd6e8ea0-7aa7-4c26-835d-6d72bebd4547"
	for _, tc := range []struct {
		name string
		cmd  func() *cobra.Command
	}{
		{"patrol", unitPatrolCmd},
		{"post", unitPostCmd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			round3Client(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					_ = json.NewDecoder(r.Body).Decode(&body)
				}
				fmt.Fprint(w, `{}`)
			})
			c := tc.cmd()
			c.SetArgs([]string{"--unit", unitID, "--q", "57", "--r", "26"})
			if _, err := captureStdout(t, c.Execute); err != nil {
				t.Fatal(err)
			}
			if body["target_q"] != float64(57) || body["target_r"] != float64(26) {
				t.Fatalf("march body = %v, want target_q=57 target_r=26", body)
			}
		})
	}
}
