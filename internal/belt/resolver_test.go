package belt

import "testing"

func TestResolveExactVillage(t *testing.T) {
	match := Resolve("Buhari", "Near Virpor", "")
	if match.Belt != "Buhari" || match.Village != "Buhari" {
		t.Fatalf("expected Buhari/Buhari from area-first rule, got %#v", match)
	}
}

func TestResolveFuzzyVillage(t *testing.T) {
	match := Resolve("Bajipura", "Tokarvaa road", "")
	if match.Belt != "Bajipura" || match.Village != "Tokarva" {
		t.Fatalf("expected Tokarva/Bajipura, got %#v", match)
	}
}

func TestResolveAvoidsValodTie(t *testing.T) {
	match := Resolve("Valod", "Vedchi faliya", "")
	if match.Village != "Vedchhi" || match.Belt != "Rupvada" {
		t.Fatalf("expected Vedchhi/Rupvada, got %#v", match)
	}
}

func TestResolvePrefersNonValodAreaVillage(t *testing.T) {
	match := Resolve("AT. KANJOD TAL VALOD", "valod bus stand", "")
	if match.Village != "Kanajod" || match.Belt != "Bhimpor" {
		t.Fatalf("expected Kanajod/Bhimpor from area-first rule, got %#v", match)
	}
}

func TestResolveMovesValodAGToShiker(t *testing.T) {
	match := Resolve("Valod", "valod bus stand", "HT line AG feeder complaint")
	if match.Village != "Valod" || match.Belt != "Shiker" {
		t.Fatalf("expected Valod/Shiker override, got %#v", match)
	}
}

func TestResolvePrefersKanjodOverTalValod(t *testing.T) {
	match := Resolve("AT. KANJOD TAL VALOD", "KANJOD", "")
	if match.Village != "Kanajod" || match.Belt != "Bhimpor" {
		t.Fatalf("expected Kanajod/Bhimpor, got %#v", match)
	}
}

func TestResolveKeepsStrongValodMatchOverWeakFuzzyMatch(t *testing.T) {
	match := Resolve("", "valod POOL FALIYA ANAND VIHAR NI SAME, VALOD", "")
	if match.Village != "Valod" || match.Belt != "Valod (T)" {
		t.Fatalf("expected Valod/Valod (T), got %#v", match)
	}
}

func TestResolveFallsBackToValodWhenLocHasNoOtherVillage(t *testing.T) {
	match := Resolve("Valod", "bus stand main road", "")
	if match.Village != "Valod" || match.Belt != "Valod (T)" {
		t.Fatalf("expected Valod/Valod (T) fallback, got %#v", match)
	}
}
