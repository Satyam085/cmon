// Package summary handles generating summary images for pending complaints.
package summary

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/fogleman/gg"
)

// Complaint holds the fields displayed in the summary table.
type Complaint struct {
	ComplainNo        string `json:"complain_no"`
	Name              string `json:"name"`
	ConsumerNo        string `json:"consumer_no"`
	MobileNo          string `json:"mobile_no"`
	Address           string `json:"address"`
	Area              string `json:"area"`
	Description       string `json:"description"`
	ComplainDate      string `json:"complain_date"`
	TelegramMessageID string `json:"telegram_message_id"`
	WhatsAppMessageID string `json:"whatsapp_message_id"`
	// APIID is the internal backend ID used for API calls (e.g. resolve).
	// Included in the JSON payload for the dashboard resolve feature.
	APIID string `json:"api_id"`

	// AgeMinutes is now() − ComplainDate at the moment the complaint was
	// fetched. Zero when ComplainDate is empty or unparseable. Surfaces as a
	// human-readable "3d 4h" cell in the dashboard + summary image so the ops
	// team can triage by how long a ticket has been pending.
	AgeMinutes int64 `json:"age_minutes"`
}

// AgeString renders an AgeMinutes value as a compact human-readable string
// like "3d 4h", "5h", or "12m". Returns "" for non-positive ages so the
// dashboard / summary image can leave the cell blank rather than print "0m".
func (c *Complaint) AgeString() string {
	return formatAge(c.AgeMinutes)
}

// formatAge converts a duration in minutes into the compact form used by the
// dashboard and summary image. Top unit is days; we never print weeks because
// the operational SLA is hours-to-days.
func formatAge(minutes int64) string {
	if minutes <= 0 {
		return ""
	}
	d := minutes / (60 * 24)
	h := (minutes % (60 * 24)) / 60
	m := minutes % 60
	switch {
	case d > 0 && h > 0:
		return fmt.Sprintf("%dd %dh", d, h)
	case d > 0:
		return fmt.Sprintf("%dd", d)
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// computeAgeMinutes returns the age in minutes for a complaint date string.
// Returns 0 when the date is empty or unparseable so callers can store the
// raw zero value without special-casing.
func computeAgeMinutes(complainDate string, now time.Time) int64 {
	t, ok := parseComplaintDate(complainDate)
	if !ok {
		return 0
	}
	delta := now.Sub(t)
	if delta < 0 {
		return 0
	}
	return int64(delta / time.Minute)
}

// renderScale is a global oversampling factor. Telegram converts photos to
// JPEG and resizes for in-chat display; rendering at a higher resolution
// gives the compressor more detail to work with, so post-compression text
// stays sharp instead of blurring. Bump to 3 if 2 still isn't enough.
const renderScale = 2

// Table styling constants. All values are post-scale (i.e. fontSize 52 means
// 26pt logical, doubled). Derive from renderScale so the relationship is
// visible at a glance and a single edit retunes everything.
const (
	cellPaddingX  = 20 * renderScale
	cellPaddingY  = 16 * renderScale
	minRowHeight  = 76 * renderScale
	headerHeight  = 88 * renderScale
	fontSize      = 26 * renderScale
	headerFontSz  = 26 * renderScale
	titleFontSz   = 40 * renderScale
	titlePadding  = 110 * renderScale
	footerPadding = 80 * renderScale
	minColWidth   = 110 * renderScale
	maxAddrWidth  = 360.0 * renderScale
	maxDescWidth  = 440.0 * renderScale
)

// Light theme colors
var (
	bgColor         = color.RGBA{R: 245, G: 247, B: 250, A: 255} // Light gray bg
	titleColor      = color.RGBA{R: 30, G: 41, B: 59, A: 255}    // Dark slate
	headerBgColor   = color.RGBA{R: 37, G: 99, B: 235, A: 255}   // Blue
	headerTextColor = color.RGBA{R: 255, G: 255, B: 255, A: 255} // White
	rowEvenColor    = color.RGBA{R: 255, G: 255, B: 255, A: 255} // White
	rowOddColor     = color.RGBA{R: 241, G: 245, B: 249, A: 255} // Subtle blue-gray
	textColor       = color.RGBA{R: 30, G: 41, B: 59, A: 255}    // Dark slate
	borderColor     = color.RGBA{R: 203, G: 213, B: 225, A: 255} // Slate border
	footerColor     = color.RGBA{R: 100, G: 116, B: 139, A: 255} // Muted slate
)

// column definition for the table.
type column struct {
	header   string
	field    func(c *Complaint) string
	maxWidth float64 // 0 means auto
}

// columns defines the table layout.
var columns = []column{
	{"Complaint No.", func(c *Complaint) string { return c.ComplainNo }, 0},
	{"Name", func(c *Complaint) string { return c.Name }, 0},
	{"Consumer No", func(c *Complaint) string { return c.ConsumerNo }, 0},
	{"Mobile No", func(c *Complaint) string { return c.MobileNo }, 0},
	{"Address", func(c *Complaint) string { return c.Address }, maxAddrWidth},
	{"Area", func(c *Complaint) string { return c.Area }, 0},
	{"Description", func(c *Complaint) string { return c.Description }, maxDescWidth},
	{"Date", func(c *Complaint) string { return c.ComplainDate }, 0},
	{"Age", func(c *Complaint) string { return c.AgeString() }, 0},
}

func encodeImage(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}
	return buf.Bytes(), nil
}

func complaintDateLess(a, b Complaint) bool {
	at, aok := parseComplaintDate(a.ComplainDate)
	bt, bok := parseComplaintDate(b.ComplainDate)
	if aok && bok {
		if at.Equal(bt) {
			return a.ComplainNo < b.ComplainNo
		}
		return at.Before(bt)
	}
	if aok != bok {
		return aok
	}
	if a.ComplainDate == b.ComplainDate {
		return a.ComplainNo < b.ComplainNo
	}
	return a.ComplainDate < b.ComplainDate
}

func parseComplaintDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}

	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"02-01-2006 15:04:05",
		"02-01-2006 15:04",
		"02-01-2006",
		"02/01/2006 15:04:05",
		"02/01/2006 15:04",
		"02/01/2006",
	}
	for _, layout := range layouts {
		if ts, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

// findFont locates a font file across Linux and Windows paths.
// It walks candidates in order and returns the first path that exists on disk.
// Returns ("", error) if no candidate is found so the caller can surface a
// useful error instead of getting a misleading "file not found" for the first
// candidate path.
func findFont(bold bool) (string, error) {
	var candidates []string
	if runtime.GOOS == "windows" {
		winRoot := os.Getenv("WINDIR")
		if winRoot == "" {
			winRoot = `C:\Windows`
		}
		if bold {
			candidates = []string{
				winRoot + `\Fonts\arialbd.ttf`,
				winRoot + `\Fonts\Arial Bold.ttf`,
			}
		} else {
			candidates = []string{
				winRoot + `\Fonts\arial.ttf`,
				winRoot + `\Fonts\Arial.ttf`,
			}
		}
	} else {
		if bold {
			candidates = []string{
				"/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf", // Fedora
				"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",   // Debian/Ubuntu
				"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",               // Arch
				"/usr/share/fonts/dejavu/DejaVuSans-Bold.ttf",            // Fedora/RHEL alt
			}
		} else {
			candidates = []string{
				"/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf", // Fedora
				"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",   // Debian/Ubuntu
				"/usr/share/fonts/TTF/DejaVuSans.ttf",               // Arch
				"/usr/share/fonts/dejavu/DejaVuSans.ttf",            // Fedora/RHEL alt
			}
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no font found; tried: %s", strings.Join(candidates, ", "))
}

// wrapText splits text into multiple lines to fit within maxWidth.
func wrapText(dc *gg.Context, text string, maxWidth float64) []string {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.TrimSpace(text)

	if maxWidth <= 0 {
		return []string{text}
	}

	w, _ := dc.MeasureString(text)
	if w <= maxWidth {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	currentLine := words[0]

	for _, word := range words[1:] {
		testLine := currentLine + " " + word
		tw, _ := dc.MeasureString(testLine)
		if tw > maxWidth {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine = testLine
		}
	}
	lines = append(lines, currentLine)
	return lines
}

// computeRowHeights calculates the height of each row based on wrapped text.
func computeRowHeights(dc *gg.Context, complaints []Complaint, colWidths []float64) []float64 {
	_, lineH := dc.MeasureString("Ay")
	lineSpacing := lineH + float64(4*renderScale)

	heights := make([]float64, len(complaints))
	for rowIdx, c := range complaints {
		c := c
		maxLines := 1
		for i, col := range columns {
			text := col.field(&c)
			innerWidth := colWidths[i] - cellPaddingX*2
			wrapped := wrapText(dc, text, innerWidth)
			if len(wrapped) > maxLines {
				maxLines = len(wrapped)
			}
		}
		h := float64(maxLines)*lineSpacing + cellPaddingY*2
		if h < float64(minRowHeight) {
			h = float64(minRowHeight)
		}
		heights[rowIdx] = h
	}
	return heights
}

// RenderTable renders all pending complaints as a single flat table image
// sorted oldest-first by complain date.
func RenderTable(complaints []Complaint) ([]byte, error) {
	if len(complaints) == 0 {
		return nil, fmt.Errorf("no complaints to render")
	}

	complaints = SortComplaints(complaints)

	boldFont, err := findFont(true)
	if err != nil {
		return nil, fmt.Errorf("failed to load bold font: %w", err)
	}
	regularFont, err := findFont(false)
	if err != nil {
		return nil, fmt.Errorf("failed to load regular font: %w", err)
	}

	// ---- Step 1: Measure column widths ----
	tmpDC := gg.NewContext(1, 1)
	if err := tmpDC.LoadFontFace(boldFont, headerFontSz); err != nil {
		return nil, fmt.Errorf("failed to load bold font: %w", err)
	}

	colWidths := make([]float64, len(columns))
	for i, col := range columns {
		w, _ := tmpDC.MeasureString(col.header)
		colWidths[i] = w + cellPaddingX*2 + 4*renderScale
		if colWidths[i] < float64(minColWidth) {
			colWidths[i] = float64(minColWidth)
		}
	}

	// Measure data widths (capped by maxWidth)
	if err := tmpDC.LoadFontFace(regularFont, fontSize); err != nil {
		return nil, fmt.Errorf("failed to load regular font: %w", err)
	}
	for _, c := range complaints {
		c := c
		for i, col := range columns {
			w, _ := tmpDC.MeasureString(col.field(&c))
			needed := w + cellPaddingX*2 + 4*renderScale
			if needed > colWidths[i] {
				colWidths[i] = needed
			}
		}
	}

	for i, col := range columns {
		if col.maxWidth > 0 && colWidths[i] > col.maxWidth {
			colWidths[i] = col.maxWidth
		}
	}

	rowHeights := computeRowHeights(tmpDC, complaints, colWidths)
	var totalRowHeight float64
	for _, h := range rowHeights {
		totalRowHeight += h
	}

	var totalWidth float64
	for _, w := range colWidths {
		totalWidth += w
	}

	canvasWidth := totalWidth + float64(40*renderScale*2)
	canvasHeight := float64(titlePadding) +
		float64(headerHeight) +
		totalRowHeight +
		float64(footerPadding)

	dc := gg.NewContext(int(canvasWidth), int(canvasHeight))

	dc.SetColor(bgColor)
	dc.Clear()

	dc.LoadFontFace(boldFont, titleFontSz)
	dc.SetColor(titleColor)
	title := fmt.Sprintf("Pending Complaints Summary  —  %s", time.Now().Format("02 Jan 2006, 03:04 PM"))
	dc.DrawStringAnchored(title, canvasWidth/2, float64(titlePadding)/2+float64(2*renderScale), 0.5, 0.5)

	tableX := float64(40 * renderScale)
	tableY := float64(titlePadding)

	dc.SetColor(headerBgColor)
	dc.DrawRoundedRectangle(tableX, tableY, totalWidth, float64(headerHeight), float64(16*renderScale))
	dc.Fill()

	dc.LoadFontFace(boldFont, headerFontSz)
	dc.SetColor(headerTextColor)
	x := tableX
	for i, col := range columns {
		tx := x + colWidths[i]/2
		ty := tableY + float64(headerHeight)/2
		dc.DrawStringAnchored(col.header, tx, ty, 0.5, 0.5)
		x += colWidths[i]
	}

	dc.LoadFontFace(regularFont, fontSize)
	_, lineH := dc.MeasureString("Ay")
	lineSpacing := lineH + float64(4*renderScale)
	curY := tableY + float64(headerHeight)

	for rowIdx, c := range complaints {
		c := c
		rh := rowHeights[rowIdx]

		if rowIdx%2 == 0 {
			dc.SetColor(rowEvenColor)
		} else {
			dc.SetColor(rowOddColor)
		}
		dc.DrawRectangle(tableX, curY, totalWidth, rh)
		dc.Fill()

		dc.SetColor(borderColor)
		dc.SetLineWidth(0.5 * renderScale)
		dc.DrawLine(tableX, curY+rh, tableX+totalWidth, curY+rh)
		dc.Stroke()

		dc.LoadFontFace(regularFont, fontSize)
		dc.SetColor(textColor)
		x := tableX
		for i, col := range columns {
			text := col.field(&c)
			innerWidth := colWidths[i] - cellPaddingX*2
			wrapped := wrapText(dc, text, innerWidth)

			totalTextH := float64(len(wrapped)) * lineSpacing
			startY := curY + (rh-totalTextH)/2 + lineH

			for lineIdx, line := range wrapped {
				ly := startY + float64(lineIdx)*lineSpacing
				dc.DrawString(line, x+cellPaddingX, ly)
			}
			x += colWidths[i]
		}

		curY += rh
	}

	dc.SetColor(borderColor)
	dc.SetLineWidth(1 * renderScale)
	totalTableH := float64(headerHeight) + totalRowHeight
	dc.DrawRoundedRectangle(tableX, tableY, totalWidth, totalTableH, float64(16*renderScale))
	dc.Stroke()

	dc.SetLineWidth(0.5 * renderScale)
	x = tableX
	for i := 0; i < len(columns)-1; i++ {
		x += colWidths[i]
		dc.DrawLine(x, tableY+float64(headerHeight), x, tableY+totalTableH)
		dc.Stroke()
	}

	dc.LoadFontFace(regularFont, 24*renderScale)
	dc.SetColor(footerColor)
	footer := fmt.Sprintf("Total: %d pending complaints", len(complaints))
	dc.DrawStringAnchored(footer, canvasWidth/2, canvasHeight-float64(30*renderScale), 0.5, 0.5)

	return encodeImage(dc.Image())
}
