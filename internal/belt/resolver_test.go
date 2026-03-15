package belt

import "testing"

func TestResolveExactVillage(t *testing.T) {
	match := Resolve("Buhari", "Near Virpor", "")
	if match.Belt != "Buhari" || match.Village != "Virpor" {
		t.Fatalf("expected Virpor/Buhari, got %#v", match)
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
