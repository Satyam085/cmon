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
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fogleman/gg"
)

// Complaint holds the fields displayed in the summary table.
type Complaint struct {
	ComplainNo   string
	Name         string
	ConsumerNo   string
	MobileNo     string
	Address      string
	Area         string
	Village      string
	Belt         string
	Description  string
	ComplainDate string
}

// Table styling constants — rendered at 2x scale for Telegram clarity
const (
	cellPaddingX  = 20
	cellPaddingY  = 16
	minRowHeight  = 76
	headerHeight  = 88
	groupHeaderH  = 64
	fontSize      = 26
	headerFontSz  = 26
	titleFontSz   = 40
	titlePadding  = 110
	footerPadding = 80
	minColWidth   = 110
	maxAddrWidth  = 360.0
	maxDescWidth  = 440.0
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

type complaintGroup struct {
	belt       string
	complaints []Complaint
}

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
	lineSpacing := lineH + 4

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

// RenderTable renders the complaints as a table image and returns PNG bytes.
func RenderTable(complaints []Complaint) ([]byte, error) {
	if len(complaints) == 0 {
		return nil, fmt.Errorf("no complaints to render")
	}

	groups := groupComplaints(complaints)

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
		colWidths[i] = w + cellPaddingX*2 + 4
		if colWidths[i] < float64(minColWidth) {
			colWidths[i] = float64(minColWidth)
		}
	}

	// Measure data widths (capped by maxWidth)
	if err := tmpDC.LoadFontFace(regularFont, fontSize); err != nil {
		return nil, fmt.Errorf("failed to load regular font: %w", err)
	}
	for _, group := range groups {
		for _, c := range group.complaints {
			c := c
			for i, col := range columns {
				w, _ := tmpDC.MeasureString(col.field(&c))
				needed := w + cellPaddingX*2 + 4
				if needed > colWidths[i] {
					colWidths[i] = needed
				}
			}
		}
	}

	// Apply max width caps
	for i, col := range columns {
		if col.maxWidth > 0 && colWidths[i] > col.maxWidth {
			colWidths[i] = col.maxWidth
		}
	}

	// Compute row heights (for text wrapping)
	rowHeightsByGroup := make([][]float64, len(groups))
	var totalRowHeight float64
	for i, group := range groups {
		rowHeightsByGroup[i] = computeRowHeights(tmpDC, group.complaints, colWidths)
		totalRowHeight += float64(groupHeaderH)
		for _, h := range rowHeightsByGroup[i] {
			totalRowHeight += h
		}
	}

	// ---- Step 2: Calculate canvas size ----
	var totalWidth float64
	for _, w := range colWidths {
		totalWidth += w
	}

	canvasWidth := totalWidth + 80 // 40px margin each side
	canvasHeight := float64(titlePadding) +
		float64(headerHeight) +
		totalRowHeight +
		float64(footerPadding)

	// ---- Step 3: Draw ----
	dc := gg.NewContext(int(canvasWidth), int(canvasHeight))

	// Background
	dc.SetColor(bgColor)
	dc.Clear()

	// Title
	dc.LoadFontFace(boldFont, titleFontSz)
	dc.SetColor(titleColor)
	title := fmt.Sprintf("Pending Complaints Summary Valod SDn  —  %s", time.Now().Format("02 Jan 2006, 03:04 PM"))
	dc.DrawStringAnchored(title, canvasWidth/2, float64(titlePadding)/2+2, 0.5, 0.5)

	tableX := 40.0
	tableY := float64(titlePadding)

	// Header row background (rounded top corners)
	dc.SetColor(headerBgColor)
	dc.DrawRoundedRectangle(tableX, tableY, totalWidth, float64(headerHeight), 16)
	dc.Fill()

	// Header text
	dc.LoadFontFace(boldFont, headerFontSz)
	dc.SetColor(headerTextColor)
	x := tableX
	for i, col := range columns {
		tx := x + colWidths[i]/2
		ty := tableY + float64(headerHeight)/2
		dc.DrawStringAnchored(col.header, tx, ty, 0.5, 0.5)
		x += colWidths[i]
	}

	// Data rows
	dc.LoadFontFace(regularFont, fontSize)
	_, lineH := dc.MeasureString("Ay")
	lineSpacing := lineH + 4
	curY := tableY + float64(headerHeight)

	rowIdx := 0
	for groupIdx, group := range groups {
		drawGroupHeader(dc, boldFont, tableX, curY, totalWidth, group.belt, len(group.complaints))
		curY += float64(groupHeaderH)

		for complaintIdx, c := range group.complaints {
			c := c
			rh := rowHeightsByGroup[groupIdx][complaintIdx]

			if rowIdx%2 == 0 {
				dc.SetColor(rowEvenColor)
			} else {
				dc.SetColor(rowOddColor)
			}
			dc.DrawRectangle(tableX, curY, totalWidth, rh)
			dc.Fill()

			dc.SetColor(borderColor)
			dc.SetLineWidth(0.5)
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
			rowIdx++
		}
	}

	// Outer table border
	dc.SetColor(borderColor)
	dc.SetLineWidth(1)
	totalTableH := float64(headerHeight) + totalRowHeight
	dc.DrawRoundedRectangle(tableX, tableY, totalWidth, totalTableH, 16)
	dc.Stroke()

	// Vertical column borders
	dc.SetLineWidth(0.5)
	x = tableX
	for i := 0; i < len(columns)-1; i++ {
		x += colWidths[i]
		dc.DrawLine(x, tableY+float64(headerHeight), x, tableY+totalTableH)
		dc.Stroke()
	}

	// Footer
	dc.LoadFontFace(regularFont, 24)
	dc.SetColor(footerColor)
	footer := fmt.Sprintf("Total: %d pending complaints", len(complaints))
	dc.DrawStringAnchored(footer, canvasWidth/2, canvasHeight-30, 0.5, 0.5)

	// ---- Step 4: Encode to PNG ----
	return encodeImage(dc.Image())
}

func encodeImage(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}
	return buf.Bytes(), nil
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxLen {
		runes := []rune(s)
		return string(runes[:maxLen]) + "…"
	}
	return s
}

func drawBeltCell(dc *gg.Context, x, y, width, height float64, belt string) {
	label := belt
	if strings.TrimSpace(label) == "" {
		label = "Unknown"
	}

	fill, text := beltColors(label)
	padX := 16.0
	padY := 12.0
	pillW := width - cellPaddingX*2
	if pillW < 40 {
		pillW = width - padX*2
	}
	pillH := height - padY*2
	if pillH > 40 {
		pillH = 40
	}
	py := y + (height-pillH)/2
	px := x + cellPaddingX

	dc.SetColor(fill)
	dc.DrawRoundedRectangle(px, py, pillW, pillH, 14)
	dc.Fill()

	dc.SetColor(text)
	dc.DrawStringAnchored(label, px+pillW/2, py+pillH/2+1, 0.5, 0.5)
}

func beltColors(belt string) (color.Color, color.Color) {
	switch strings.ToLower(strings.TrimSpace(belt)) {
	case "buhari":
		return color.RGBA{R: 220, G: 252, B: 231, A: 255}, color.RGBA{R: 22, G: 101, B: 52, A: 255}
	case "rupvada":
		return color.RGBA{R: 219, G: 234, B: 254, A: 255}, color.RGBA{R: 30, G: 64, B: 175, A: 255}
	case "bhimpor":
		return color.RGBA{R: 254, G: 240, B: 138, A: 255}, color.RGBA{R: 133, G: 77, B: 14, A: 255}
	case "shiker":
		return color.RGBA{R: 233, G: 213, B: 255, A: 255}, color.RGBA{R: 107, G: 33, B: 168, A: 255}
	case "bajipura":
		return color.RGBA{R: 254, G: 215, B: 170, A: 255}, color.RGBA{R: 154, G: 52, B: 18, A: 255}
	case "kelkui":
		return color.RGBA{R: 204, G: 251, B: 241, A: 255}, color.RGBA{R: 17, G: 94, B: 89, A: 255}
	case "valod (t)":
		return color.RGBA{R: 254, G: 226, B: 226, A: 255}, color.RGBA{R: 153, G: 27, B: 27, A: 255}
	default:
		return color.RGBA{R: 226, G: 232, B: 240, A: 255}, color.RGBA{R: 51, G: 65, B: 85, A: 255}
	}
}

func groupComplaints(complaints []Complaint) []complaintGroup {
	grouped := make(map[string][]Complaint)
	for _, complaint := range complaints {
		belt := strings.TrimSpace(complaint.Belt)
		if belt == "" {
			belt = "Unknown"
			complaint.Belt = belt
		}
		grouped[belt] = append(grouped[belt], complaint)
	}

	groups := make([]complaintGroup, 0, len(grouped))
	for belt, items := range grouped {
		sort.Slice(items, func(i, j int) bool {
			return complaintDateLess(items[i], items[j])
		})
		groups = append(groups, complaintGroup{belt: belt, complaints: items})
	}

	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].complaints) == 0 || len(groups[j].complaints) == 0 {
			return groups[i].belt < groups[j].belt
		}
		left := groups[i].complaints[0]
		right := groups[j].complaints[0]
		if complaintDateLess(left, right) {
			return true
		}
		if complaintDateLess(right, left) {
			return false
		}
		return groups[i].belt < groups[j].belt
	})

	return groups
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

func drawGroupHeader(dc *gg.Context, boldFont string, x, y, width float64, belt string, count int) {
	fill, text := beltColors(belt)
	dc.SetColor(fill)
	dc.DrawRectangle(x, y, width, float64(groupHeaderH))
	dc.Fill()

	dc.SetColor(borderColor)
	dc.SetLineWidth(0.5)
	dc.DrawLine(x, y+float64(groupHeaderH), x+width, y+float64(groupHeaderH))
	dc.Stroke()

	dc.LoadFontFace(boldFont, headerFontSz-2)
	dc.SetColor(text)
	label := fmt.Sprintf("%s Belt  •  %d complaints", belt, count)
	dc.DrawString(label, x+cellPaddingX, y+float64(groupHeaderH)/2+10)
}
