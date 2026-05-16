package summary

import "sort"

// SortComplaints orders complaints oldest-first with a ComplainNo tie-break.
// Used by the summary image and the dashboard so every view stays consistent.
func SortComplaints(complaints []Complaint) []Complaint {
	out := append([]Complaint(nil), complaints...)
	sort.Slice(out, func(i, j int) bool {
		return complaintDateLess(out[i], out[j])
	})
	return out
}
