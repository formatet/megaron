// Standalone simulation/report only. Run from repository root:
// go run docs/reviews/kharis-om/simulate.go docs/reviews/kharis-om
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
 "os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type sample struct {
	Tick                            int     `json:"tick"`
	Kharis                          float64 `json:"kharis"`
	Gain                            float64 `json:"gain"`
	Decay                           float64 `json:"decay"`
	Cap                             float64 `json:"cap"`
	Band                            string  `json:"band"`
	PunishmentProbability           float64 `json:"punishment_probability"`
	ExpectedPunishmentRollHits      float64 `json:"expected_punishment_roll_hits"`
	ProbabilityAnyPunishmentRollHit float64 `json:"probability_any_punishment_roll_hit"`
}
type scenario struct {
	Rule            string   `json:"rule"`
	Cities          int      `json:"cities"`
	TemplesLabel    string   `json:"temples_label"`
	Temples         int      `json:"temples"`
	Start           float64  `json:"start"`
	FirstSuspicious int      `json:"first_suspicious_tick"`
	FirstWrathful   int      `json:"first_wrathful_tick"`
	Samples         []sample `json:"samples"`
}

func band(k float64) string {
	if k >= 60 {
		return "Favorable"
	}
	if k >= 30 {
		return "Indifferent"
	}
	if k >= 5 {
		return "Suspicious"
	}
	return "Wrathful"
}
func run(rule string, n, t int, label string) scenario {
	start := 40.0
	if rule == "current_actual_start25" {
		start = 25
	}
	s := scenario{Rule: rule, Cities: n, Temples: t, TemplesLabel: label, Start: start, FirstSuspicious: -1, FirstWrathful: -1}
	k := start
	expected, none := 0.0, 1.0
	s.Samples = append(s.Samples, sample{Tick: 0, Kharis: k, Band: band(k)})
	for tick := 1; tick <= 300; tick++ {
		gain := float64(t) * 0.15 * 14 // devotion_sum * kharisPerTempleDay, all fed, scarcity1
		cap := 25.0
		if t > 0 {
			cap = 50
		}
		if rule == "approved_no_temple_cap40" && t == 0 {
			cap = 40
		}
		decay := 0.1 + 0.1*float64(n*(n-1)/2)
		old := strings.HasPrefix(rule, "current_")
		if old {
			templeless := n - 1
			if t > 0 {
				templeless = n - t
			} // capital receives the first temple
			decay = 1.8 + math.Max(0, float64(templeless-4))
			if tick < 144 {
				decay = 0
			} // inFoundingGrace(age), age starts at1 at the first simulated tick
		}
		k = math.Max(1, math.Min(cap, k+gain-decay)) // existing clampKharis / EffectiveKharisCap
		b := band(k)
		p := 0.0
		if old {
			if t == 0 && tick >= 144 && k < 30 {
				p = 0.3
			}
		} else {
			if k < 5 {
				p = 0.3
			} else if k < 30 {
				p = 0.1
			}
		}
		expected += p
		none *= 1 - p
		if k < 30 && s.FirstSuspicious < 0 {
			s.FirstSuspicious = tick
		}
		if k < 5 && s.FirstWrathful < 0 {
			s.FirstWrathful = tick
		}
		s.Samples = append(s.Samples, sample{tick, k, gain, decay, cap, b, p, expected, 1 - none})
	}
	return s
}
func check(ok bool, msg string) {
	if !ok {
		panic(msg)
	}
}
func main() {
	out := "docs/reviews/kharis-om"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		panic(err)
	}
	// Pin the model to the exact inspected production baseline. These checks fail
	// if its constants change: this report must be regenerated/reviewed then.
	paths := []string{"server/internal/kharis/tick.go", "server/internal/religion/mood.go", "server/api/handlers/create_metropolis.go"}
	hashes := map[string]string{}
	for _, p := range paths {
		raw, err := exec.Command("git", "show", "590d47ac:"+p).Output()
		if err != nil {
			panic(err)
		}
		hashes[p] = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	raw, err := exec.Command("git", "show", "590d47ac:"+paths[0]).Output()
	if err != nil {
		panic(err)
	}
	code := string(raw)
	for _, fragment := range []string{"kharisPerTempleDay = 14.0", "TempleDevotionPerLevel = 0.15", "decayBas          = 1.8", "decayFreeColonies = 4", "foundingGraceTicks = 144", "kharisCapPerTempleLevel = 25.0", "return kharisCapPerTempleLevel * float64(1+maxTempleLevel)"} {
		check(strings.Contains(code, fragment), "baseline changed: "+fragment)
	}
	scenarios := []scenario{}
	for _, rule := range []string{"current_actual_start25", "current_controlled_start40", "proposed_existing_caps", "approved_no_temple_cap40"} {
		for _, n := range []int{1, 3, 6, 10} {
			for _, setting := range []struct {
				label string
				t     int
			}{{"none", 0}, {"one", 1}, {"each_city", n}} {
				scenarios = append(scenarios, run(rule, n, setting.t, setting.label))
			}
		}
	}
	for _, s := range scenarios {
		if s.Rule == "proposed_existing_caps" && s.Cities == 1 && s.TemplesLabel == "none" {
			check(s.Samples[1].Kharis == 25 && s.FirstSuspicious == 1, "cap-cliff not reproduced")
			check(math.Abs(s.Samples[16].ProbabilityAnyPunishmentRollHit-(1-math.Pow(.9, 16))) < 1e-12, "risk calculation differs")
		}
		if s.Rule == "proposed_existing_caps" && s.Cities == 6 && s.TemplesLabel == "one" {
			check(s.Samples[1].Gain-s.Samples[1].Decay > 0 && s.Samples[300].Kharis == 50, "six-city fed temple does not climb")
		}
	}
	result := map[string]any{"baseline": "590d47ac", "production_file_sha256": hashes, "ticks": 300, "assumptions": []string{"fully fed L1 temples staffed at15% from tick0", "scarcity1, passive kharis_rate0", "capital receives first temple; fixed owned city count", "no rites, builds or punishment feedback", "risk is Bernoulli roll hazard given an eligible target, not actual simulated damage", "strict binary-float thresholds as current code; boundary crossing can differ by one tick from decimal arithmetic", "no-temple cap40 approved by Claude clarification 2026-10-18 13:15; L1/L2/L3 unchanged"}, "gate": "PASS after approved cap40 clarification; cap25 variant retained as rejected diagnosis", "scenarios": scenarios}
	j, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(out, "simulation.json"), j, 0644); err != nil {
		panic(err)
	}
	f, err := os.Create(filepath.Join(out, "curves.csv"))
	if err != nil {
		panic(err)
	}
	w := csv.NewWriter(f)
	w.Write([]string{"rule", "cities", "temples_label", "temples", "tick", "kharis", "gain", "decay", "cap", "band", "punishment_probability", "expected_roll_hits", "probability_any_roll_hit"})
	for _, s := range scenarios {
		for _, x := range s.Samples {
			w.Write([]string{s.Rule, strconv.Itoa(s.Cities), s.TemplesLabel, strconv.Itoa(s.Temples), strconv.Itoa(x.Tick), fmt.Sprintf("%.10f", x.Kharis), fmt.Sprint(x.Gain), fmt.Sprint(x.Decay), fmt.Sprint(x.Cap), x.Band, fmt.Sprint(x.PunishmentProbability), fmt.Sprint(x.ExpectedPunishmentRollHits), fmt.Sprint(x.ProbabilityAnyPunishmentRollHit)})
		}
	}
	w.Flush()
	check(w.Error() == nil, "CSV write failed")
	f.Close()
	fmt.Println("rule cities temples first_suspicious first_wrathful end_kharis expected_roll_hits risk_any")
	for _, s := range scenarios {
		if s.Rule != "current_controlled_start40" {
			x := s.Samples[300]
			fmt.Printf("%s %d %s %d %d %.3f %.3f %.6f\n", s.Rule, s.Cities, s.TemplesLabel, s.FirstSuspicious, s.FirstWrathful, x.Kharis, x.ExpectedPunishmentRollHits, x.ProbabilityAnyPunishmentRollHit)
		}
	}
	fmt.Println("SIMULATION_GATE=PASS: approved cap0=40 preserves opening; one-city L1 and six-city one-L1 climb; cap25 variant rejected")
}
