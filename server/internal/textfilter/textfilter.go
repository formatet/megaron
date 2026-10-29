// Package textfilter replaces coarse words in text a Wanax writes for other
// Wanaxes to read (Timothy 2026-10-29): in a letter the word becomes
// comic-strip grawlix ("!?@#"), in a name a made-up word in the style of a
// Linear B transliteration, so a map label still reads as a place. The word
// is replaced, never refused, and the original is not kept.
//
// Only the coarse is listed (sexual vulgarity, slurs, hate words) in English
// and Swedish; mild swearing passes. Lists: words_en.txt, words_sv.txt.
// Zero internal dependencies (G1 bottom tier).
//
// Matching is per word, after folding digits and look-alike letters
// ("sh1t", Cyrillic "а") and with each letter allowed to repeat ("fuuuck").
// A run of single letters ("f u c k", "f.u.c.k") is read as one word. It is a
// speed bump, not a wall; whole-word entries keep Scunthorpe out.
package textfilter

import (
	"embed"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed words_en.txt words_sv.txt
var lists embed.FS

var whole, prefix, contains = load()

func load() (w, p, c *regexp.Regexp) {
	var ws, ps, cs []string
	for _, f := range []string{"words_en.txt", "words_sv.txt"} {
		b, err := lists.ReadFile(f)
		if err != nil {
			panic(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "" || line[0] == '#':
			case len(line) > 2 && line[0] == '*' && line[len(line)-1] == '*':
				cs = append(cs, pattern(line[1:len(line)-1]))
			case line[len(line)-1] == '*':
				ps = append(ps, pattern(line[:len(line)-1]))
			default:
				ws = append(ws, pattern(line))
			}
		}
	}
	return regexp.MustCompile(`^(?:` + strings.Join(ws, "|") + `)$`),
		regexp.MustCompile(`^(?:` + strings.Join(ps, "|") + `)`),
		regexp.MustCompile(strings.Join(cs, "|"))
}

// pattern lets every run of a letter repeat: "fitta" matches "fiittta" but
// not "fita", so "profitable" stays clean.
func pattern(word string) string {
	var b strings.Builder
	rs := []rune(normalize(word))
	for i := 0; i < len(rs); {
		j := i
		for j < len(rs) && rs[j] == rs[i] {
			j++
		}
		b.WriteString(regexp.QuoteMeta(string(rs[i])))
		if j-i == 1 {
			b.WriteString("+")
		} else {
			b.WriteString("{" + itoa(j-i) + ",}")
		}
		i = j
	}
	return b.String()
}

func itoa(n int) string { return string(rune('0' + n)) }

var fold = map[rune]rune{
	'0': 'o', '1': 'i', '3': 'e', '4': 'a', '5': 's', '7': 't', '@': 'a', '$': 's',
	// Cyrillic and Greek look-alikes.
	'а': 'a', 'е': 'e', 'о': 'o', 'р': 'p', 'с': 'c', 'у': 'y', 'х': 'x', 'і': 'i', 'к': 'k', 'т': 't', 'н': 'h',
	'α': 'a', 'ε': 'e', 'ο': 'o', 'ρ': 'p', 'κ': 'k', 'τ': 't', 'ι': 'i', 'ν': 'v',
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if f, ok := fold[r]; ok {
			r = f
		}
		b.WriteRune(r)
	}
	return b.String()
}

func coarse(word string) bool {
	k := normalize(word)
	return whole.MatchString(k) || prefix.MatchString(k) || contains.MatchString(k)
}

func wordRune(r rune) bool {
	_, folded := fold[r]
	return unicode.IsLetter(r) || unicode.IsDigit(r) || folded
}

type span struct{ start, end int } // byte offsets into the text

// coarseSpans finds each coarse word, joining runs of three or more single
// letters split only by spaces or punctuation.
func coarseSpans(s string) []span {
	var words []span
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if !wordRune(r) {
			i += n
			continue
		}
		j := i
		for j < len(s) {
			r, n := utf8.DecodeRuneInString(s[j:])
			if !wordRune(r) {
				break
			}
			j += n
		}
		words = append(words, span{i, j})
		i = j
	}
	var hits []span
	for i := 0; i < len(words); {
		j := i
		var joined strings.Builder
		for j < len(words) && utf8.RuneCountInString(s[words[j].start:words[j].end]) == 1 &&
			(j == i || words[j].start-words[j-1].end <= 2) {
			joined.WriteString(s[words[j].start:words[j].end])
			j++
		}
		if j-i >= 3 && coarse(joined.String()) {
			hits = append(hits, span{words[i].start, words[j-1].end})
			i = j
			continue
		}
		if coarse(s[words[i].start:words[i].end]) {
			hits = append(hits, words[i])
		}
		i++
	}
	return hits
}

func replace(s string, intn func(int) int, with func(word string, intn func(int) int) string) (string, int) {
	hits := coarseSpans(s)
	if len(hits) == 0 {
		return s, 0
	}
	var b strings.Builder
	last := 0
	for _, h := range hits {
		b.WriteString(s[last:h.start])
		b.WriteString(with(s[h.start:h.end], intn))
		last = h.end
	}
	b.WriteString(s[last:])
	return b.String(), len(hits)
}

// Letter replaces each coarse word with grawlix of the same length and
// returns how many words it replaced.
func Letter(s string, intn func(int) int) (string, int) {
	return replace(s, intn, grawlix)
}

// Name replaces each coarse word with a made-up Linear B-style word of the
// same length, capitalised like the original, and returns how many words it
// replaced. The length is kept so a name that passed its limits still does.
func Name(s string, intn func(int) int) (string, int) {
	return replace(s, intn, linearB)
}

const grawlixRunes = "!?@#$%&*"

func grawlix(word string, intn func(int) int) string {
	n := utf8.RuneCountInString(word)
	out := make([]byte, n)
	for i := range out {
		c := grawlixRunes[intn(len(grawlixRunes))]
		for i > 0 && c == out[i-1] {
			c = grawlixRunes[intn(len(grawlixRunes))]
		}
		out[i] = c
	}
	return string(out)
}

// syllables are open syllables of the Linear B syllabary, as transliterated.
var syllables = strings.Fields(`a e i o u da de di do du ja je jo ka ke ki ko ku
	ma me mi mo mu na ne ni no nu pa pe pi po pu qa qe qi qo ra re ri ro ru
	sa se si so su ta te ti to tu wa we wi wo za ze zo`)

func linearB(word string, intn func(int) int) string {
	n := utf8.RuneCountInString(word)
	for {
		var b strings.Builder
		for b.Len() < n {
			b.WriteString(syllables[intn(len(syllables))])
		}
		out := b.String()[:n]
		if coarse(out) {
			continue
		}
		if r, _ := utf8.DecodeRuneInString(word); unicode.IsUpper(r) {
			out = strings.ToUpper(out[:1]) + out[1:]
		}
		return out
	}
}
