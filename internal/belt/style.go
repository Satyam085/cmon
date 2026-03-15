package belt

import (
	"image/color"
	"strings"
)

var knownBelts = []string{
	"Buhari",
	"Rupvada",
	"Bhimpor",
	"Shiker",
	"Bajipura",
	"Kelkui",
	"Valod (T)",
}

var canonicalBelts = map[string]string{
	"buhari":   "Buhari",
	"rupvada":  "Rupvada",
	"bhimpor":  "Bhimpor",
	"shiker":   "Shiker",
	"bajipura": "Bajipura",
	"kelkui":   "Kelkui",
	"valod":    "Valod (T)",
	"valodt":   "Valod (T)",
}

type Style struct {
	Label string
	Emoji string
	Fill  color.Color
	Text  color.Color
}

func All() []string {
	out := make([]string, len(knownBelts))
	copy(out, knownBelts)
	return out
}

func Canonicalize(name string) (string, bool) {
	key := beltKey(name)
	if key == "" {
		return "", false
	}

	canonical, ok := canonicalBelts[key]
	return canonical, ok
}

func DisplayName(name string) string {
	if canonical, ok := Canonicalize(name); ok {
		return canonical
	}

	label := strings.TrimSpace(name)
	if label == "" {
		return "Unknown"
	}
	return label
}

func MessageLabel(name string) string {
	style := StyleFor(name)
	return style.Emoji + " " + style.Label
}

func StyleFor(name string) Style {
	label := DisplayName(name)

	switch strings.ToLower(label) {
	case "buhari":
		return Style{
			Label: label,
			Emoji: "🟢",
			Fill:  color.RGBA{R: 220, G: 252, B: 231, A: 255},
			Text:  color.RGBA{R: 22, G: 101, B: 52, A: 255},
		}
	case "rupvada":
		return Style{
			Label: label,
			Emoji: "🔵",
			Fill:  color.RGBA{R: 219, G: 234, B: 254, A: 255},
			Text:  color.RGBA{R: 30, G: 64, B: 175, A: 255},
		}
	case "bhimpor":
		return Style{
			Label: label,
			Emoji: "🟡",
			Fill:  color.RGBA{R: 254, G: 240, B: 138, A: 255},
			Text:  color.RGBA{R: 133, G: 77, B: 14, A: 255},
		}
	case "shiker":
		return Style{
			Label: label,
			Emoji: "🟣",
			Fill:  color.RGBA{R: 233, G: 213, B: 255, A: 255},
			Text:  color.RGBA{R: 107, G: 33, B: 168, A: 255},
		}
	case "bajipura":
		return Style{
			Label: label,
			Emoji: "🟠",
			Fill:  color.RGBA{R: 254, G: 215, B: 170, A: 255},
			Text:  color.RGBA{R: 154, G: 52, B: 18, A: 255},
		}
	case "kelkui":
		return Style{
			Label: label,
			Emoji: "🟤",
			Fill:  color.RGBA{R: 254, G: 243, B: 199, A: 255},
			Text:  color.RGBA{R: 146, G: 64, B: 14, A: 255},
		}
	case "valod (t)":
		return Style{
			Label: label,
			Emoji: "🔴",
			Fill:  color.RGBA{R: 254, G: 226, B: 226, A: 255},
			Text:  color.RGBA{R: 153, G: 27, B: 27, A: 255},
		}
	default:
		return Style{
			Label: label,
			Emoji: "⚪",
			Fill:  color.RGBA{R: 226, G: 232, B: 240, A: 255},
			Text:  color.RGBA{R: 51, G: 65, B: 85, A: 255},
		}
	}
}

func beltKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}

	return b.String()
}
