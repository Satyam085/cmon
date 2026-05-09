// bridge.go wires the whatsapp package to the summary, api, and storage packages.
//
// This file exists to avoid circular imports: the whatsapp package imports
// summary and api here (at the edges of the call graph), while client.go
// uses function variables (fetchPendingSummary, renderSummaryImage,
// renderSummaryImages, resolveComplaintAPI) that point to the functions
// defined here.
package whatsapp

import (
	"fmt"

	"cmon/internal/api"
	"cmon/internal/session"
	"cmon/internal/storage"
	"cmon/internal/summary"
)

// summaryComplaint mirrors summary.Complaint locally so client.go doesn't import summary.
type summaryComplaint = summary.Complaint

// summaryBeltImage mirrors summary.BeltImage so client.go doesn't import summary.
type summaryBeltImage = summary.BeltImage

// fetchSummaryComplaints calls summary.FetchAllPendingDetails.
// storI must be *storage.Storage.
func fetchSummaryComplaints(sc *session.Client, storI summaryStorage) ([]summaryComplaint, error) {
	stor, ok := storI.(*storage.Storage)
	if !ok {
		return nil, fmt.Errorf("storage type mismatch in fetchSummaryComplaints")
	}
	complaints, err := summary.FetchAllPendingDetails(sc, stor)
	if err != nil {
		return nil, fmt.Errorf("summary fetch: %w", err)
	}
	return complaints, nil
}

// renderTable calls summary.RenderTable (combined image with belt group headers).
func renderTable(complaints []summaryComplaint) ([]byte, error) {
	return summary.RenderTable(complaints)
}

// renderTablesByBelt calls summary.RenderTablesByBelt (one image per belt).
func renderTablesByBelt(complaints []summaryComplaint) ([]summaryBeltImage, error) {
	return summary.RenderTablesByBelt(complaints)
}

// resolveOnWebsite calls api.ResolveComplaint.
func resolveOnWebsite(sc *session.Client, apiID, remark string, debugMode bool) error {
	return api.ResolveComplaint(sc, apiID, remark, debugMode)
}
