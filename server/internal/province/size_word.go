package province

// SizeWord is the one owner of how big another ruler's city reads from
// outside (megaron_kontrakt_frammande_stad, Timothy 2026-10-30): a word, never
// the number. Web and keryx take the word from the server and never derive it.
// Thresholds are calibration (decided 10-30, beside the wall ceilings).
func SizeWord(population int) string {
	switch {
	case population < 1000:
		return "hamlet"
	case population < 5000:
		return "town"
	case population < 10000:
		return "city"
	default:
		return "great city"
	}
}
