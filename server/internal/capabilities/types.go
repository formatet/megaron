// Package capabilities is the server-side "what can I do + what does it take"
// surface (temenos_capabilities.md). It answers, for a given province, which
// mutating verbs are currently usable and — for the locked ones — exactly what
// live gap stands in the way (detail) and how to close it (hint).
//
// G1 placement: capabilities sits ABOVE the domain packages (province, unit,
// religion, clock) and is consumed by api/handlers, which sits higher still.
// It imports domain packages for their types/constants but talks to the
// database directly (mirroring — not calling — the handlers' own query
// logic), per the Fas 1 build plan. Fas 3 unifies handler validation with
// these checkers so 422s and the capabilities list can never drift apart.
package capabilities

// Verb describes one mutating action: whether it is available right now, and
// if not, exactly what is missing.
type Verb struct {
	Name         string        `json:"name"`
	Category     string        `json:"category"`
	Purpose      string        `json:"purpose"`
	Available    bool          `json:"available"`
	Requirements []Requirement `json:"requirements"`
}

// Requirement is one gate a verb depends on. Text is the static requirement
// ("a deployable land unit"); Detail is the live gap ("0/1 deployable");
// Hint is the actionable next step ("recruit 100 men in one settlement").
type Requirement struct {
	Text      string `json:"text"`
	Satisfied bool   `json:"satisfied"`
	Detail    string `json:"detail"`
	Hint      string `json:"hint"`
}

// The six locked categories (temenos_capabilities.md — "Kategori-taxonomi").
// Do not add new categories; fold new verbs into one of these.
const (
	CategoryProvince  = "province"
	CategoryMilitary  = "military"
	CategoryTrade     = "trade"
	CategoryDiplomacy = "diplomacy"
	CategoryKingdom   = "kingdom"
	CategoryCult      = "cult"
)

// req is a small constructor to keep checker bodies terse.
func req(text string, satisfied bool, detail, hint string) Requirement {
	return Requirement{Text: text, Satisfied: satisfied, Detail: detail, Hint: hint}
}

// verb builds a Verb, computing Available as the AND of every requirement.
// A verb with no requirements is trivially available (F3 — still listed).
func verb(name, category, purpose string, reqs []Requirement) Verb {
	if reqs == nil {
		reqs = []Requirement{}
	}
	available := true
	for _, r := range reqs {
		if !r.Satisfied {
			available = false
			break
		}
	}
	return Verb{Name: name, Category: category, Purpose: purpose, Available: available, Requirements: reqs}
}
