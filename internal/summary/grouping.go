package summary

// ComplaintGroup is the grouped, sorted representation used by the summary
// image and by the web dashboard.
type ComplaintGroup struct {
	Belt       string      `json:"belt"`
	Complaints []Complaint `json:"complaints"`
}

// GroupComplaints applies the same grouping and sort rules used by the summary
// image so every view stays consistent.
func GroupComplaints(complaints []Complaint) []ComplaintGroup {
	grouped := groupComplaints(complaints)
	out := make([]ComplaintGroup, 0, len(grouped))
	for _, group := range grouped {
		out = append(out, ComplaintGroup{
			Belt:       group.belt,
			Complaints: group.complaints,
		})
	}
	return out
}
