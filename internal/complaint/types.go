// Package complaint provides types and structures for complaint data.
package complaint

// Link represents a complaint link extracted from the dashboard table.
//
// Fields:
//   - ComplaintNumber: Display number shown to users (e.g., "12345")
//   - APIID: Internal ID used for API calls (e.g., "456")
//
// Why two IDs:
//   - ComplaintNumber: User-facing, shown in Telegram messages
//   - APIID: Backend ID, used for API calls to fetch details/resolve
type Link struct {
	ComplaintNumber string
	APIID           string
}

// Details represents the full complaint information from the API.
//
// This struct uses interface{} for flexibility because the API
// sometimes returns null values or different types.
//
// Fields map to API response JSON:
//   - complain_no: Complaint number
//   - consumer_no: Consumer account number
//   - complainant_name: Name of person who filed complaint
//   - mobile_no: Contact phone number
//   - description: Detailed complaint text
//   - complain_date: When complaint was filed
//   - exact_location: Specific location of issue
//   - area: General area/locality
type Details struct {
	ComplainNo      interface{} `json:"complain_no"`
	ConsumerNo      interface{} `json:"consumer_no"`
	ComplainantName interface{} `json:"complainant_name"`
	MobileNo        interface{} `json:"mobile_no"`
	Description     interface{} `json:"description"`
	ComplainDate    interface{} `json:"complain_date"`
	ExactLocation   interface{} `json:"exact_location"`
	Area            interface{} `json:"area"`
}

// ProcessResult represents the result of processing a single complaint.
//
// This is used in the worker pool to collect results from concurrent
// complaint processing operations.
//
// Fields:
//   - ComplaintID: Complaint number that was processed
//   - MessageID: Telegram message ID (empty if send failed)
//   - ConsumerName: Name extracted from complaint details
//   - Error: Any error that occurred during processing
type ProcessResult struct {
	ComplaintID  string
	MessageID    string
	ConsumerName string
	Error        error
}
