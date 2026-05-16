package summary

import (
	"testing"
	"time"
)

// TestParseComplaintDate covers every accepted layout plus the rejection
// path. The function is the single place that turns the DGVCL date strings
// into Time values, and the rest of the sort logic depends on it
// classifying things correctly.
func TestParseComplaintDate(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantOK  bool
		wantStr string // expected parsed time formatted as 2006-01-02 15:04:05; "" if wantOK is false
	}{
		{"empty", "", false, ""},
		{"whitespace only", "   ", false, ""},
		{"iso datetime seconds", "2026-03-04 10:11:12", true, "2026-03-04 10:11:12"},
		{"iso datetime minutes", "2026-03-04 10:11", true, "2026-03-04 10:11:00"},
		{"iso date only", "2026-03-04", true, "2026-03-04 00:00:00"},
		{"dmy dash datetime seconds", "04-03-2026 10:11:12", true, "2026-03-04 10:11:12"},
		{"dmy dash date only", "04-03-2026", true, "2026-03-04 00:00:00"},
		{"dmy slash datetime", "04/03/2026 10:11:12", true, "2026-03-04 10:11:12"},
		{"dmy slash date only", "04/03/2026", true, "2026-03-04 00:00:00"},
		{"surrounding whitespace", "  2026-03-04  ", true, "2026-03-04 00:00:00"},
		{"unrecognised layout", "March 4, 2026", false, ""},
		{"garbage", "not a date", false, ""},
		{"impossible date", "2026-13-45", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseComplaintDate(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parseComplaintDate(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.Format("2006-01-02 15:04:05") != tc.wantStr {
				t.Errorf("parseComplaintDate(%q) = %s, want %s",
					tc.in, got.Format("2006-01-02 15:04:05"), tc.wantStr)
			}
		})
	}
}

func TestComplaintDateLess(t *testing.T) {
	t.Run("both parseable: earlier date wins", func(t *testing.T) {
		a := Complaint{ComplainNo: "9", ComplainDate: "2026-03-01"}
		b := Complaint{ComplainNo: "1", ComplainDate: "2026-03-02"}
		if !complaintDateLess(a, b) {
			t.Error("earlier date should sort first regardless of complaint number")
		}
		if complaintDateLess(b, a) {
			t.Error("later date should not sort before earlier")
		}
	})

	t.Run("equal dates: lower complaint number wins", func(t *testing.T) {
		a := Complaint{ComplainNo: "5", ComplainDate: "2026-03-01"}
		b := Complaint{ComplainNo: "9", ComplainDate: "2026-03-01"}
		if !complaintDateLess(a, b) {
			t.Error("lower ComplainNo should sort first when dates are equal")
		}
		if complaintDateLess(b, a) {
			t.Error("higher ComplainNo should not sort before lower")
		}
	})

	t.Run("only one parseable: parseable wins", func(t *testing.T) {
		ok := Complaint{ComplainNo: "1", ComplainDate: "2026-03-01"}
		bad := Complaint{ComplainNo: "1", ComplainDate: "garbage"}
		if !complaintDateLess(ok, bad) {
			t.Error("complaint with parseable date should sort before unparseable")
		}
		if complaintDateLess(bad, ok) {
			t.Error("complaint with unparseable date should not sort before parseable")
		}
	})

	t.Run("both unparseable: lexical fall-back on date string then number", func(t *testing.T) {
		a := Complaint{ComplainNo: "1", ComplainDate: "alpha"}
		b := Complaint{ComplainNo: "1", ComplainDate: "beta"}
		if !complaintDateLess(a, b) {
			t.Error("expected lexical date order when neither parses")
		}

		c := Complaint{ComplainNo: "5", ComplainDate: "alpha"}
		d := Complaint{ComplainNo: "9", ComplainDate: "alpha"}
		if !complaintDateLess(c, d) {
			t.Error("expected ComplainNo tiebreak when both have same unparseable date")
		}
	})
}

// TestSortComplaints locks in the public ordering: oldest first, then
// ComplainNo as the tie-break, and parseable dates winning over unparseable.
func TestSortComplaints(t *testing.T) {
	in := []Complaint{
		{ComplainNo: "9", ComplainDate: "2026-03-03"},
		{ComplainNo: "5", ComplainDate: "2026-03-01"},
		{ComplainNo: "7", ComplainDate: "2026-03-01"}, // tie with #5; lower No first
	}
	got := SortComplaints(in)

	wantOrder := []string{"5", "7", "9"}
	if len(got) != len(wantOrder) {
		t.Fatalf("len: got %d, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got[i].ComplainNo != want {
			t.Errorf("position %d: got %s, want %s", i, got[i].ComplainNo, want)
		}
	}
}

func TestSortComplaintsEmptyInput(t *testing.T) {
	if got := SortComplaints(nil); len(got) != 0 {
		t.Errorf("expected zero results for nil input, got %d", len(got))
	}

	if got := SortComplaints([]Complaint{}); len(got) != 0 {
		t.Errorf("expected zero results for empty input, got %d", len(got))
	}
}

// TestParseComplaintDateUsesLocalLocation guards against a future change that
// accidentally parses portal dates as UTC. The portal emits dates in IST and
// downstream sort expects parsing in time.Local.
func TestParseComplaintDateUsesLocalLocation(t *testing.T) {
	got, ok := parseComplaintDate("2026-03-04 10:11:12")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	want := time.Date(2026, 3, 4, 10, 11, 12, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %s in %s, want %s in %s",
			got, got.Location(), want, want.Location())
	}
}

func TestFormatAge(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, ""},
		{-5, ""},
		{1, "1m"},
		{59, "59m"},
		{60, "1h"},
		{75, "1h 15m"},
		{60 * 23, "23h"},
		{60 * 24, "1d"},
		{60*24 + 30, "1d"},      // less than an hour past day boundary → no h component
		{60*25 + 0, "1d 1h"},    // 25h
		{60*72 + 60*4, "3d 4h"}, // 3d 4h
		{60 * 24 * 7, "7d"},     // 1 week
	}
	for _, tc := range cases {
		t.Run(formatAge(tc.in), func(t *testing.T) {
			if got := formatAge(tc.in); got != tc.want {
				t.Errorf("formatAge(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestComputeAgeMinutes(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.Local)

	t.Run("two hours ago", func(t *testing.T) {
		got := computeAgeMinutes("2026-05-10 10:00:00", now)
		if got != 120 {
			t.Errorf("got %d, want 120", got)
		}
	})

	t.Run("future date returns zero", func(t *testing.T) {
		got := computeAgeMinutes("2026-05-11 12:00:00", now)
		if got != 0 {
			t.Errorf("future date should yield 0, got %d", got)
		}
	})

	t.Run("unparseable returns zero", func(t *testing.T) {
		if got := computeAgeMinutes("not a date", now); got != 0 {
			t.Errorf("unparseable date should yield 0, got %d", got)
		}
	})

	t.Run("empty returns zero", func(t *testing.T) {
		if got := computeAgeMinutes("", now); got != 0 {
			t.Errorf("empty should yield 0, got %d", got)
		}
	})
}
