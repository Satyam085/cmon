package belt

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

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

func TestPopulateBeltCSV(t *testing.T) {
	if os.Getenv("BELT_CSV_UPDATE") != "1" {
		t.Skip("set BELT_CSV_UPDATE=1 to populate belt_text.csv")
	}

	csvPath := findBeltCSVPath(t)

	file, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("open csv: %v", err)
	}
	defer file.Close()

	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("csv is empty: %s", csvPath)
	}

	ensureCols := func(row []string, want int) []string {
		for len(row) < want {
			row = append(row, "")
		}
		return row
	}

	records[0] = ensureCols(records[0], 4)
	records[0][0] = "area"
	records[0][1] = "loc"
	records[0][2] = "belt"
	records[0][3] = "score"

	for i := 1; i < len(records); i++ {
		records[i] = ensureCols(records[i], 4)
		match := resolve(records[i][0], records[i][1], "")
		records[i][2] = match.Belt
		records[i][3] = strconv.Itoa(match.score)
	}

	out, err := os.Create(csvPath)
	if err != nil {
		t.Fatalf("create csv: %v", err)
	}
	defer out.Close()

	writer := csv.NewWriter(out)
	if err := writer.WriteAll(records); err != nil {
		t.Fatalf("write csv: %v", err)
	}
}

func findBeltCSVPath(t *testing.T) string {
	t.Helper()

	candidates := []string{
		"belt_text.csv",
		filepath.Join("..", "..", "belt_text.csv"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Fatalf("could not find belt_text.csv")
	return ""
}
