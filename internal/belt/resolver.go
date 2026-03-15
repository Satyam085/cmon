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
	{Village: "Kanjod", Belt: "Bhimpor"},
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
	normVillage string // pre-normalized village name for scoring
	score       int
}

// normalizedEntries pre-computes the normalized village name for each entry
// so that normalize() is not called on every Resolve invocation.
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

// Resolve returns the best village and belt match using the complaint area,
// location, and description.
//
// Valod is treated as a last resort: if any other village clears the minimum
// score threshold (72), it is preferred over Valod regardless of the score gap.
// This prevents ambiguous area/location strings that partially match "Valod"
// from overriding a real village match.
func Resolve(area, loc, desc string) Entry {
	segments := buildSegments(area, loc)
	if len(segments) == 0 {
		return Entry{}
	}

	best := scoredEntry{}
	bestNonValod := scoredEntry{}

	for _, entry := range normalizedEntries {
		score := scoreEntry(entry.normVillage, segments)
		if score > best.score {
			best = scoredEntry{Entry: entry.Entry, normVillage: entry.normVillage, score: score}
		}
		// Track the best non-Valod match independently so that even if Valod
		// happens to score highest, we can fall back to the real best match.
		if !strings.EqualFold(entry.Village, "Valod") && score > bestNonValod.score {
			bestNonValod = scoredEntry{Entry: entry.Entry, normVillage: entry.normVillage, score: score}
		}
	}

	if best.score < 72 {
		return Entry{}
	}

	// If Valod is the top scorer but any other village also clears the
	// threshold, prefer that village instead.
	if strings.EqualFold(best.Village, "Valod") && bestNonValod.score >= 72 {
		best = bestNonValod
	}

	// AG (Agriculture) feeders in the Valod town belt actually belong to the
	// Shiker belt. Override the belt when the description signals AG.
	if strings.EqualFold(best.Belt, "Valod (T)") && hasAG(desc) {
		return Entry{Village: best.Village, Belt: "Shiker"}
	}

	return best.Entry
}

func scoreEntry(normVillage string, segments []segment) int {
	if normVillage == "" {
		return 0
	}

	best := 0
	for _, seg := range segments {
		score := similarityScore(normVillage, seg.value) + seg.weight
		if score > best {
			best = score
		}
	}
	return best
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
// Note: this is intentionally asymmetric — it checks that the village's tokens
// are a subset of the segment's tokens, not the other way around. This allows
// multi-word village names like "Pelad Buhari" to match a longer segment that
// contains both words.
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

// hasAG reports whether the description contains an "AG" token, which signals
// an Agriculture feeder. Handles common encodings:
//   - "AG" or "ag"       → normalized to token "ag"
//   - "A.G." or "A/G"   → normalized to adjacent tokens "a" "g"
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

// minInt returns the smallest of the provided integers.
// Named minInt to avoid shadowing the Go 1.21 builtin min.
func minInt(vals ...int) int {
	best := vals[0]
	for _, v := range vals[1:] {
		if v < best {
			best = v
		}
	}
	return best
}

// maxInt returns the larger of two integers.
// Named maxInt to avoid shadowing the Go 1.21 builtin max.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}