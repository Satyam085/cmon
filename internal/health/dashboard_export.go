package health

// Export endpoints for the dashboard. exportRow is a flat per-complaint
// projection used by both /export.json (direct) and /export.csv (via
// csvRecord).

import (
	"strconv"
	"time"

	"cmon/internal/session"
	"cmon/internal/storage"
)

// exportRow is the flat per-complaint shape returned by the export
// endpoints.
type exportRow struct {
	ComplainNo        string `json:"complain_no"`
	Name              string `json:"name"`
	ConsumerNo        string `json:"consumer_no"`
	MobileNo          string `json:"mobile_no"`
	Address           string `json:"address"`
	Area              string `json:"area"`
	Description       string `json:"description"`
	ComplainDate      string `json:"complain_date"`
	AgeMinutes        int64  `json:"age_minutes"`
	Age               string `json:"age"`
	APIID             string `json:"api_id"`
	TelegramMessageID string `json:"telegram_message_id"`
	WhatsAppMessageID string `json:"whatsapp_message_id"`
}

// exportCSVHeader keeps the CSV column order in lock-step with csvRecord
// below. Edit them together.
var exportCSVHeader = []string{
	"complain_no",
	"name",
	"consumer_no",
	"mobile_no",
	"address",
	"area",
	"description",
	"complain_date",
	"age_minutes",
	"age",
	"api_id",
	"telegram_message_id",
	"whatsapp_message_id",
}

func (r exportRow) csvRecord() []string {
	return []string{
		r.ComplainNo,
		r.Name,
		r.ConsumerNo,
		r.MobileNo,
		r.Address,
		r.Area,
		r.Description,
		r.ComplainDate,
		strconv.FormatInt(r.AgeMinutes, 10),
		r.Age,
		r.APIID,
		r.TelegramMessageID,
		r.WhatsAppMessageID,
	}
}

// buildExportRows runs the same fetch the dashboard uses and flattens the
// result into a slice of rows ready for CSV/JSON.
//
// The second return value is the generated_at timestamp, exposed so the
// JSON wrapper can echo it back to the caller.
func buildExportRows(monitor *Monitor, sc *session.Client, stor *storage.Storage) ([]exportRow, string, error) {
	payload, err := buildComplaintDashboardPayload(monitor, sc, stor)
	if err != nil {
		return nil, "", err
	}

	rows := make([]exportRow, 0, len(payload.Complaints))
	for _, c := range payload.Complaints {
		rows = append(rows, exportRow{
			ComplainNo:        c.ComplainNo,
			Name:              c.Name,
			ConsumerNo:        c.ConsumerNo,
			MobileNo:          c.MobileNo,
			Address:           c.Address,
			Area:              c.Area,
			Description:       c.Description,
			ComplainDate:      c.ComplainDate,
			AgeMinutes:        c.AgeMinutes,
			Age:               c.AgeString(),
			APIID:             c.APIID,
			TelegramMessageID: c.TelegramMessageID,
			WhatsAppMessageID: c.WhatsAppMessageID,
		})
	}
	return rows, payload.GeneratedAt, nil
}

// exportFilename returns a date-stamped filename like
// cmon-complaints-2026-05-10.csv so a browser save-as defaults to a
// human-readable name. IST date because the operator works in IST.
func exportFilename(ext string) string {
	return "cmon-complaints-" + time.Now().Format("2006-01-02") + "." + ext
}
