package belt

import (
	"strings"
	"unicode"
)

type Entry struct {
	Village string
	Belt    string
}

var entries = []Entry{
	{Village: "Adyapor", Belt: "Buhari"},
	{Village: "Ambach", Belt: "Rupvada"},
	{Village: "Andhatri", Belt: "Buhari"},
	{Village: "Bahej", Belt: "Bhimpor"},
	{Village: "Bhimpor", Belt: "Bhimpor"},
	{Village: "Bhojpur Najik", Belt: "Rupvada"},
	{Village: "Boriya", Belt: "Shiker"},
	{Village: "Borkhadi", Belt: "Bajipura"},
	{Village: "Bajipura", Belt: "Bajipura"},
	{Village: "Buhari", Belt: "Buhari"},
	{Village: "Butwada", Belt: "Bajipura"},
	{Village: "Dadariya", Belt: "Buhari"},
	{Village: "Degama", Belt: "Rupvada"},
	{Village: "Delwada", Belt: "Shiker"},
	{Village: "Dharampura", Belt: "Buhari"},
	{Village: "Dumkhal", Belt: "Bhimpor"},
	{Village: "Gheriyavav", Belt: "Kelkui"},
	{Village: "Goddha", Belt: "Buhari"},
	{Village: "Golan", Belt: "Bhimpor"},
	{Village: "Hathuka", Belt: "Bhimpor"},
	{Village: "Inama", Belt: "Bajipura"},
	{Village: "Kalakva", Belt: "Buhari"},
	{Village: "Kamalchhod", Belt: "Bajipura"},
	{Village: "Kanajod", Belt: "Bhimpor"},
	{Village: "Kasvav", Belt: "Kelkui"},
	{Village: "Kelkui", Belt: "Kelkui"},
	{Village: "Khambhla", Belt: "Shiker"},
	{Village: "Khanpur", Belt: "Rupvada"},
	{Village: "Kosambiya", Belt: "Bhimpor"},
	{Village: "Kumbhiya", Belt: "Bhimpor"},
	{Village: "Nalotha", Belt: "Bhimpor"},
	{Village: "Nansad", Belt: "Shiker"},
	{Village: "Pelad Buhari", Belt: "Buhari"},
	{Village: "Ranveri", Belt: "Bhimpor"},
	{Village: "Rupvada", Belt: "Rupvada"},
	{Village: "Sejvad", Belt: "Shiker"},
	{Village: "Shahpor", Belt: "Shiker"},
	{Village: "Shiker", Belt: "Shiker"},
	{Village: "Titva", Belt: "Bajipura"},
	{Village: "Tokarva", Belt: "Bajipura"},
	{Village: "Umarkachchh", Belt: "Buhari"},
	{Village: "Valod", Belt: "Valod (T)"},
	{Village: "Vedchhi", Belt: "Rupvada"},
	{Village: "Virpor", Belt: "Buhari"},
}

type scoredEntry struct {
	Entry
	normVillage string
	score       int
}

// normalizedEntries pre-computes the normalized village name for each entry.
var normalizedEntries = func() []scoredEntry {
	out := make([]scoredEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, scoredEntry{
			Entry:       e,
			normVillage: normalize(e.Village),
		})
	}
	return out
}()

type segment struct {
	value  string
	weight int
}

const (
	minResolveScore    = 72
	strongAreaScore    = 88
	canonicalValod     = "Valod"
	canonicalValodBelt = "Valod (T)"
)

// Resolve returns the best village and belt match using the complaint area,
// location, and description.
//
// Resolution order:
//  1. Area strongly matches a non-Valod village (score >= 88) -> use that village.
//  2. Area is exactly "Valod" -> search loc for another village; fallback to Valod (T).
//  3. General case -> score area + loc together, Valod excluded entirely.
func Resolve(area, loc, desc string) Entry {
	return resolve(area, loc, desc).Entry
}

func resolve(area, loc, desc string) scoredEntry {
	normArea := normalize(area)

	// Step 1: strong area match (non-Valod).
	if match, ok := resolveStrongArea(normArea); ok {
		return match
	}

	// Step 2: area is explicitly Valod.
	if normArea == normalize(canonicalValod) {
		if match, ok := resolveBest(loc, false); ok {
			return match
		}
		valodScore := bestMatch(buildSegments("", normArea), true).score
		if hasAG(desc) {
			return scoredEntry{
				Entry: Entry{Village: canonicalValod, Belt: "Shiker"},
				score: valodScore,
			}
		}
		return scoredEntry{
			Entry: Entry{Village: canonicalValod, Belt: canonicalValodBelt},
			score: valodScore,
		}
	}

	// Step 3: general case — score everything, but still avoid letting Valod
	// override a real non-Valod village when both are plausible.
	segments := buildSegments(area, loc)
	if len(segments) == 0 {
		return scoredEntry{}
	}

	best := bestMatch(segments, true)
	bestNonValod := bestMatch(segments, false)
	if best.score < minResolveScore {
		return scoredEntry{}
	}
	if strings.EqualFold(best.Village, canonicalValod) && bestNonValod.score >= minResolveScore && bestNonValod.score >= best.score-20 {
		best = bestNonValod
	}

	// AG feeders in the Valod town belt belong to the Shiker belt.
	if strings.EqualFold(best.Belt, canonicalValodBelt) && hasAG(desc) {
		return scoredEntry{
			Entry: Entry{Village: best.Village, Belt: "Shiker"},
			score: best.score,
		}
	}

	return best
}

// resolveStrongArea returns a match when the area field alone scores >= strongAreaScore
// against a non-Valod village. The area text is scored with loc-like weight so
// village mentions embedded inside larger area strings still surface strongly.
func resolveStrongArea(normArea string) (scoredEntry, bool) {
	if normArea == "" || normArea == normalize(canonicalValod) {
		return scoredEntry{}, false
	}

	best := bestMatch(buildSegments("", normArea), false)
	if best.score < strongAreaScore {
		return scoredEntry{}, false
	}
	return best, true
}

// resolveBest searches a single text string and returns the best match
// above minResolveScore.
func resolveBest(text string, includeValod bool) (scoredEntry, bool) {
	best := bestMatch(buildSegments("", text), includeValod)
	if best.score < minResolveScore {
		return scoredEntry{}, false
	}
	return best, true
}

// bestMatch returns the highest-scoring entry from normalizedEntries.
//
// Optimisation: since the first letter of the village name is guaranteed to be
// correct in the input, we pre-compute the set of first letters present across
// all segment words and hard-skip any entry whose village starts with a
// different letter — avoiding Levenshtein calls for obviously unrelated villages.
func bestMatch(segments []segment, includeValod bool) scoredEntry {
	// Collect every first letter present in the segments.
	firstLetters := make(map[rune]bool)
	for _, seg := range segments {
		for _, word := range strings.Fields(seg.value) {
			runes := []rune(word)
			if len(runes) > 0 {
				firstLetters[runes[0]] = true
			}
		}
	}

	best := scoredEntry{}
	for _, entry := range normalizedEntries {
		if !includeValod && strings.EqualFold(entry.Village, canonicalValod) {
			continue
		}
		// Hard skip: first letter not present anywhere in the input.
		if len(entry.normVillage) > 0 {
			vFirst := []rune(entry.normVillage)[0]
			if !firstLetters[vFirst] {
				continue
			}
		}
		score := scoreEntry(entry.normVillage, segments)
		if score > best.score {
			best = scoredEntry{Entry: entry.Entry, normVillage: entry.normVillage, score: score}
		}
	}
	return best
}

func scoreEntry(normVillage string, segments []segment) int {
	if normVillage == "" {
		return 0
	}

	best := 0
	for _, seg := range segments {
		// segmentSharesFirstLetter is a secondary guard here;
		// the primary first-letter gate lives in bestMatch.
		if !segmentSharesFirstLetter(normVillage, seg.value) {
			continue
		}
		score := similarityScore(normVillage, seg.value) + seg.weight
		if score > best {
			best = score
		}
	}
	return best
}

// segmentSharesFirstLetter reports whether any word in seg starts with the
// same rune as the first rune of village. Both inputs are already normalized.
func segmentSharesFirstLetter(village, seg string) bool {
	if village == "" || seg == "" {
		return false
	}
	vFirst := []rune(village)[0]
	for _, word := range strings.Fields(seg) {
		runes := []rune(word)
		if len(runes) > 0 && runes[0] == vFirst {
			return true
		}
	}
	return false
}

func similarityScore(village, segment string) int {
	if village == "" || segment == "" {
		return 0
	}

	if segment == village {
		return 100
	}
	if strings.Contains(segment, village) {
		return 98
	}
	if tokenSubset(village, segment) {
		return 95
	}

	dist := levenshtein(village, segment)
	maxLen := maxInt(len([]rune(village)), len([]rune(segment)))
	if maxLen == 0 {
		return 0
	}
	score := 100 - (dist*100/maxLen)
	if score < 0 {
		return 0
	}
	return score
}

// tokenSubset reports whether every token in village appears in segment.
func tokenSubset(village, segment string) bool {
	villageTokens := strings.Fields(village)
	segmentTokens := strings.Fields(segment)
	if len(villageTokens) == 0 || len(segmentTokens) == 0 {
		return false
	}

	seen := make(map[string]bool, len(segmentTokens))
	for _, token := range segmentTokens {
		seen[token] = true
	}

	for _, token := range villageTokens {
		if !seen[token] {
			return false
		}
	}
	return true
}

func buildSegments(area, loc string) []segment {
	type weightedPart struct {
		text   string
		weight int
	}

	parts := []weightedPart{
		{text: loc, weight: 16},
		{text: area, weight: 0},
		{text: loc + " " + area, weight: 4},
		{text: area + " " + loc, weight: 2},
	}

	seen := make(map[string]bool)
	var segments []segment

	for _, part := range parts {
		norm := normalize(part.text)
		if norm == "" {
			continue
		}
		addSegment(norm, part.weight, seen, &segments)

		for _, chunk := range splitChunks(norm) {
			addSegment(chunk, part.weight, seen, &segments)
		}

		words := strings.Fields(norm)
		for start := 0; start < len(words); start++ {
			for width := 1; width <= 3 && start+width <= len(words); width++ {
				addSegment(strings.Join(words[start:start+width], " "), part.weight, seen, &segments)
			}
		}
	}

	return segments
}

func splitChunks(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '/' || r == '-' || r == '|'
	})
}

func addSegment(value string, weight int, seen map[string]bool, segments *[]segment) {
	if value == "" || seen[value] {
		return
	}
	seen[value] = true
	*segments = append(*segments, segment{value: value, weight: weight})
}

// hasAG reports whether the description contains an "AG" token (Agriculture feeder).
// Handles: "AG", "ag", "A.G.", "A/G".
func hasAG(desc string) bool {
	desc = normalize(desc)
	if desc == "" {
		return false
	}

	tokens := strings.Fields(desc)
	for i, tok := range tokens {
		if tok == "ag" {
			return true
		}
		// "A.G." and "A/G" normalize to two adjacent tokens "a" and "g".
		if tok == "a" && i+1 < len(tokens) && tokens[i+1] == "g" {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))

	lastSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}

	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}

func levenshtein(a, b string) int {
	ar := []rune(a)
	br := []rune(b)

	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}

	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)

	for j := range prev {
		prev[j] = j
	}

	for i, ra := range ar {
		curr[0] = i + 1
		for j, rb := range br {
			cost := 0
			if ra != rb {
				cost = 1
			}
			insertCost := curr[j] + 1
			deleteCost := prev[j+1] + 1
			replaceCost := prev[j] + cost
			curr[j+1] = minInt(insertCost, deleteCost, replaceCost)
		}
		prev, curr = curr, prev
	}

	return prev[len(br)]
}

func minInt(vals ...int) int {
	best := vals[0]
	for _, v := range vals[1:] {
		if v < best {
			best = v
		}
	}
	return best
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
