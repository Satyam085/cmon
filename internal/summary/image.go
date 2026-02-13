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
	Description  string
	ComplainDate string
}

// Table styling constants — rendered at 2x scale for Telegram clarity
const (
	cellPaddingX  = 20
	cellPaddingY  = 16
	minRowHeight  = 76
	headerHeight  = 88
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
func findFont(bold bool) string {
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
				"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
				"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
			}
		} else {
			candidates = []string{
				"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
				"/usr/share/fonts/TTF/DejaVuSans.ttf",
			}
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return candidates[0]
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

	// Sort by complaint date ascending
	sort.Slice(complaints, func(i, j int) bool {
		return complaints[i].ComplainDate < complaints[j].ComplainDate
	})

	boldFont := findFont(true)
	regularFont := findFont(false)

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
	for _, c := range complaints {
		c := c
		for i, col := range columns {
			w, _ := tmpDC.MeasureString(col.field(&c))
			needed := w + cellPaddingX*2 + 4
			if needed > colWidths[i] {
				colWidths[i] = needed
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
	rowHeights := computeRowHeights(tmpDC, complaints, colWidths)

	// ---- Step 2: Calculate canvas size ----
	var totalWidth float64
	for _, w := range colWidths {
		totalWidth += w
	}

	var totalRowHeight float64
	for _, h := range rowHeights {
		totalRowHeight += h
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

	for rowIdx, c := range complaints {
		c := c
		rh := rowHeights[rowIdx]

		// Alternating row background
		if rowIdx%2 == 0 {
			dc.SetColor(rowEvenColor)
		} else {
			dc.SetColor(rowOddColor)
		}
		dc.DrawRectangle(tableX, curY, totalWidth, rh)
		dc.Fill()

		// Row border (bottom)
		dc.SetColor(borderColor)
		dc.SetLineWidth(0.5)
		dc.DrawLine(tableX, curY+rh, tableX+totalWidth, curY+rh)
		dc.Stroke()

		// Row text with wrapping
		dc.SetColor(textColor)
		x := tableX
		for i, col := range columns {
			text := col.field(&c)
			innerWidth := colWidths[i] - cellPaddingX*2
			wrapped := wrapText(dc, text, innerWidth)

			totalTextH := float64(len(wrapped)) * lineSpacing
			startY := curY + (rh-totalTextH)/2 + lineH // vertically center

			for lineIdx, line := range wrapped {
				ly := startY + float64(lineIdx)*lineSpacing
				dc.DrawString(line, x+cellPaddingX, ly)
			}
			x += colWidths[i]
		}

		curY += rh
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
