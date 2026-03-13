package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/go-tangra/go-tangra-paperless/cmd/server/assets"
)

// FieldFill represents a field value to fill in the PDF
type FieldFill struct {
	Value     string
	Type      string
	ImageData []byte // For signature/initials fields
	// Position on the PDF page (from template field definitions)
	PageNumber    int
	XPercent      float64
	YPercent      float64
	WidthPercent  float64
	HeightPercent float64
}

// PDFProcessor handles PDF field filling and signing page generation
type PDFProcessor struct {
	log *log.Helper
}

// NewPDFProcessor creates a new PDFProcessor
func NewPDFProcessor(ctx *bootstrap.Context) *PDFProcessor {
	l := ctx.NewLoggerHelper("paperless/service/pdf-processor")

	// Initialize pdfcpu config (sets up font.UserFontDir to ~/.config/pdfcpu/fonts)
	api.LoadConfiguration()

	// Install DejaVu Sans font for Cyrillic/Unicode support in PDF text overlays
	if font.UserFontDir == "" {
		l.Warn("pdfcpu UserFontDir not set, font installation skipped")
	} else {
		if err := font.InstallFontFromBytes(font.UserFontDir, "DejaVuSans", assets.DejaVuSansFont); err != nil {
			l.Warnf("failed to install DejaVuSans font: %v", err)
		} else {
			if err := font.LoadUserFonts(); err != nil {
				l.Warnf("failed to load user fonts: %v", err)
			} else {
				l.Infof("installed and loaded DejaVuSans font into %s (available: %v)", font.UserFontDir, font.UserFontNamesVerbose())
			}
		}
	}

	return &PDFProcessor{
		log: l,
	}
}

// FillFields applies field values and signature to a PDF.
// 1. Overlay fields at their positions on original pages
// 2. Append a signing summary page (audit trail)
func (p *PDFProcessor) FillFields(pdfBytes []byte, fills []FieldFill, signerName, signerEmail string) ([]byte, error) {
	if len(fills) == 0 {
		return pdfBytes, nil
	}

	// Step 1: Overlay fields at positions on original pages
	positionedFills := make([]FieldFill, 0)
	for _, f := range fills {
		if f.PageNumber > 0 && (f.XPercent > 0 || f.YPercent > 0) {
			positionedFills = append(positionedFills, f)
		}
	}
	if len(positionedFills) > 0 {
		result, err := p.overlayFieldsOnPages(pdfBytes, positionedFills)
		if err != nil {
			p.log.Warnf("failed to overlay fields on pages: %v", err)
		} else {
			p.log.Infof("overlaid %d fields on original pages", len(positionedFills))
			pdfBytes = result
		}
	}

	// Step 2: Append signing summary page (audit trail)
	if signerName != "" || signerEmail != "" {
		result, err := p.appendSigningPage(pdfBytes, fills, signerName, signerEmail)
		if err != nil {
			p.log.Warnf("failed to append signing page: %v", err)
			return pdfBytes, nil
		}
		p.log.Infof("signing page appended with %d field values", len(fills))
		pdfBytes = result
	}

	return pdfBytes, nil
}

// StampRevocation overlays a diagonal semi-transparent stamp (e.g. "АНУЛИРАНО") on every page of the PDF.
// The stamp is rendered as a large, rotated, red watermark text centered on each page.
func (p *PDFProcessor) StampRevocation(pdfBytes []byte, stampText string) ([]byte, error) {
	if stampText == "" {
		stampText = "АНУЛИРАНО"
	}

	// Build a diagonal red stamp watermark applied to all pages
	desc := fmt.Sprintf(
		"font:DejaVuSans, points:72, position:c, offset:0 0, scalefactor:1 abs, color:0.8 0 0, opacity:0.35, rotation:45",
	)
	wm, err := api.TextWatermark(stampText, desc, true, false, types.POINTS)
	if err != nil {
		return nil, fmt.Errorf("create revocation watermark: %w", err)
	}

	var buf bytes.Buffer
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	if err := api.AddWatermarks(bytes.NewReader(pdfBytes), &buf, nil, wm, conf); err != nil {
		return nil, fmt.Errorf("apply revocation stamp: %w", err)
	}

	p.log.Infof("applied revocation stamp '%s' to PDF", stampText)
	return buf.Bytes(), nil
}

// overlayFieldsOnPages stamps field values and signature images at detected positions
// on the original PDF pages using pdfcpu's watermark/stamp API.
func (p *PDFProcessor) overlayFieldsOnPages(pdfBytes []byte, fills []FieldFill) ([]byte, error) {
	// First pass: read PDF to get page dimensions for coordinate conversion
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := api.ReadContext(bytes.NewReader(pdfBytes), conf)
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	if err := api.OptimizeContext(ctx); err != nil {
		return nil, fmt.Errorf("optimize pdf: %w", err)
	}
	if err := ctx.XRefTable.EnsurePageCount(); err != nil {
		return nil, fmt.Errorf("ensure page count: %w", err)
	}
	p.log.Infof("overlay: PDF has %d pages", ctx.PageCount)

	// Cache page dimensions
	type pageDims struct{ width, height float64 }
	dims := make(map[int]pageDims)
	for pageNum := 1; pageNum <= ctx.PageCount; pageNum++ {
		_, _, inhPAttrs, err := ctx.XRefTable.PageDict(pageNum, false)
		if err != nil || inhPAttrs.MediaBox == nil {
			continue
		}
		dims[pageNum] = pageDims{inhPAttrs.MediaBox.Width(), inhPAttrs.MediaBox.Height()}
	}

	// Build watermark slice map: page number -> list of stamps
	// Order matters: white cover first, then actual content on top
	wmMap := make(map[int][]*model.Watermark)
	result := pdfBytes

	for _, f := range fills {
		d, ok := dims[f.PageNumber]
		if !ok {
			p.log.Warnf("overlay: no dimensions for page %d", f.PageNumber)
			continue
		}

		// Convert CSS coords (top-left origin, percentages) to PDF coords (bottom-left origin, points)
		x := f.XPercent / 100.0 * d.width
		fieldH := f.HeightPercent / 100.0 * d.height
		fieldW := f.WidthPercent / 100.0 * d.width
		yTop := f.YPercent / 100.0 * d.height // distance from top
		yBottom := d.height - yTop - fieldH    // PDF bottom-left y

		if fieldW < 20 {
			fieldW = 120
		}
		if fieldH < 10 {
			fieldH = 20
		}

		// Step 1: Stamp a white rectangle over the full placeholder area to hide the original text.
		// Create a white PNG at exact fieldW x fieldH pixels (1px = 1pt), then use scalefactor:1 abs.
		coverPNG := createWhitePNG(int(fieldW)+2, int(fieldH)+2)
		coverFile, err := os.CreateTemp("", "cover-*.png")
		if err != nil {
			p.log.Warnf("overlay: create cover temp file: %v", err)
		} else {
			coverFile.Write(coverPNG)
			coverFile.Close()
			coverDesc := fmt.Sprintf("position:bl, offset:%.1f %.1f, scalefactor:1 abs, opacity:1, rotation:0",
				x-1, yBottom-1)
			coverWM, err := api.ImageWatermark(coverFile.Name(), coverDesc, true, false, types.POINTS)
			os.Remove(coverFile.Name())
			if err != nil {
				p.log.Warnf("overlay: create cover watermark: %v", err)
			} else {
				wmMap[f.PageNumber] = append(wmMap[f.PageNumber], coverWM)
			}
		}

		// Step 2: Stamp the actual content on top
		isSignature := f.Type == "signature" || f.Type == "initials"

		if isSignature && len(f.ImageData) > 0 {
			// Write signature image to temp file for pdfcpu image watermark
			tmpFile, err := os.CreateTemp("", "sig-*.png")
			if err != nil {
				p.log.Warnf("overlay: create temp file: %v", err)
				continue
			}
			if _, err := tmpFile.Write(f.ImageData); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				p.log.Warnf("overlay: write temp image: %v", err)
				continue
			}
			tmpFile.Close()

			// Decode image to get dimensions for scaling
			img, _, err := image.Decode(bytes.NewReader(f.ImageData))
			if err != nil {
				os.Remove(tmpFile.Name())
				p.log.Warnf("overlay: decode image for dimensions: %v", err)
				continue
			}
			imgW := float64(img.Bounds().Dx())
			imgH := float64(img.Bounds().Dy())
			scaleX := fieldW / imgW
			scaleY := fieldH / imgH
			scale := scaleX
			if scaleY < scale {
				scale = scaleY
			}
			// Center the image within the field area
			drawW := imgW * scale
			drawH := imgH * scale
			offsetX := x + (fieldW-drawW)/2
			offsetY := yBottom + (fieldH-drawH)/2

			desc := fmt.Sprintf("position:bl, offset:%.1f %.1f, scalefactor:%.4f abs, opacity:1, rotation:0",
				offsetX, offsetY, scale)
			wm, err := api.ImageWatermark(tmpFile.Name(), desc, true, false, types.POINTS)
			os.Remove(tmpFile.Name())
			if err != nil {
				p.log.Warnf("overlay: create image watermark: %v", err)
				continue
			}
			wmMap[f.PageNumber] = append(wmMap[f.PageNumber], wm)
		} else if !isSignature && f.Value != "" {
			// Text stamp on top of the white cover
			fontSize := int(fieldH * 0.6)
			if fontSize > 12 {
				fontSize = 12
			}
			if fontSize < 6 {
				fontSize = 6
			}

			desc := fmt.Sprintf("font:DejaVuSans, points:%d, position:bl, offset:%.1f %.1f, scalefactor:1 abs, color:0 0 0, opacity:1, rotation:0, margins:2",
				fontSize, x, yBottom)
			wm, err := api.TextWatermark(f.Value, desc, true, false, types.POINTS)
			if err != nil {
				p.log.Warnf("overlay: create text watermark for %q: %v", f.Value, err)
				continue
			}
			wmMap[f.PageNumber] = append(wmMap[f.PageNumber], wm)
		}
	}

	if len(wmMap) == 0 {
		return pdfBytes, nil
	}

	// Apply all stamps using pdfcpu's official API
	var buf bytes.Buffer
	stampConf := model.NewDefaultConfiguration()
	stampConf.ValidationMode = model.ValidationRelaxed
	if err := api.AddWatermarksSliceMap(bytes.NewReader(result), &buf, wmMap, stampConf); err != nil {
		return nil, fmt.Errorf("add watermarks: %w", err)
	}

	return buf.Bytes(), nil
}

// appendSigningPage adds a signing summary page to the PDF using pdfcpu stamps.
// Strategy: stamp all content onto a standalone blank page PDF first, then merge it
// with the original document. This avoids page numbering issues with AddWatermarksSliceMap.
func (p *PDFProcessor) appendSigningPage(pdfBytes []byte, fills []FieldFill, signerName, signerEmail string) ([]byte, error) {
	blankPagePDF := buildBlankPagePDF()

	// Build watermarks targeting page 1 of the standalone blank page
	now := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
	wmMap := map[int][]*model.Watermark{}
	const pg = 1

	y := 750.0

	// Title
	addTextWM(wmMap, pg, "Document Signing Certificate", 18, 72, y, true)
	y -= 15

	// Separator line
	addTextWM(wmMap, pg, "________________________________________________________________________________________________________", 6, 72, y, false)
	y -= 25

	// Signer Information header
	addTextWM(wmMap, pg, "Signer Information", 11, 72, y, true)
	y -= 20

	addTextWM(wmMap, pg, fmt.Sprintf("Name: %s", signerName), 10, 72, y, false)
	y -= 16

	addTextWM(wmMap, pg, fmt.Sprintf("Email: %s", signerEmail), 10, 72, y, false)
	y -= 16

	addTextWM(wmMap, pg, fmt.Sprintf("Signed at: %s", now), 10, 72, y, false)
	y -= 30

	// Field values section
	hasFields := false
	for _, fill := range fills {
		if fill.Type != "signature" && fill.Type != "initials" && fill.Value != "" {
			hasFields = true
			break
		}
	}

	if hasFields {
		addTextWM(wmMap, pg, "Field Values", 11, 72, y, true)
		y -= 20

		for _, fill := range fills {
			if fill.Type == "signature" || fill.Type == "initials" || fill.Value == "" {
				continue
			}
			addTextWM(wmMap, pg, fmt.Sprintf("Field: %s", fill.Value), 10, 72, y, false)
			y -= 16
		}
		y -= 14
	}

	// Signature image section
	for _, fill := range fills {
		if (fill.Type == "signature" || fill.Type == "initials") && len(fill.ImageData) > 0 {
			addTextWM(wmMap, pg, "Signature", 11, 72, y, true)
			y -= 10

			tmpFile, err := os.CreateTemp("", "audit-sig-*.png")
			if err != nil {
				break
			}
			if _, err := tmpFile.Write(fill.ImageData); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				break
			}
			tmpFile.Close()

			img, _, err := image.Decode(bytes.NewReader(fill.ImageData))
			if err != nil {
				os.Remove(tmpFile.Name())
				break
			}
			imgW := float64(img.Bounds().Dx())
			imgH := float64(img.Bounds().Dy())
			maxW, maxH := 250.0, 80.0
			scale := maxW / imgW
			if imgH*scale > maxH {
				scale = maxH / imgH
			}
			drawH := imgH * scale

			desc := fmt.Sprintf("position:bl, offset:75 %.1f, scalefactor:%.4f abs, opacity:1, rotation:0", y-drawH, scale)
			wm, err := api.ImageWatermark(tmpFile.Name(), desc, true, false, types.POINTS)
			os.Remove(tmpFile.Name())
			if err == nil {
				wmMap[pg] = append(wmMap[pg], wm)
			}
			y -= drawH + 20
			break
		}
	}

	// Footer
	y -= 20
	addTextWM(wmMap, pg, "This page was automatically generated as part of the document signing process.", 8, 72, y, false)

	// Stamp all watermarks onto the blank page
	stampedPage := blankPagePDF
	if len(wmMap) > 0 {
		var stampBuf bytes.Buffer
		stampConf := model.NewDefaultConfiguration()
		stampConf.ValidationMode = model.ValidationRelaxed
		if err := api.AddWatermarksSliceMap(bytes.NewReader(blankPagePDF), &stampBuf, wmMap, stampConf); err != nil {
			return nil, fmt.Errorf("failed to stamp signing page: %w", err)
		}
		stampedPage = stampBuf.Bytes()
	}

	// Merge: original PDF + stamped signing page
	var mergeBuf bytes.Buffer
	mergeConf := model.NewDefaultConfiguration()
	mergeConf.ValidationMode = model.ValidationRelaxed
	readers := []io.ReadSeeker{
		bytes.NewReader(pdfBytes),
		bytes.NewReader(stampedPage),
	}
	if err := api.MergeRaw(readers, &mergeBuf, false, mergeConf); err != nil {
		return nil, fmt.Errorf("failed to merge signing page: %w", err)
	}

	return mergeBuf.Bytes(), nil
}

// addTextWM adds a text watermark to the watermark map
func addTextWM(wmMap map[int][]*model.Watermark, pageNum int, text string, fontSize int, x, y float64, bold bool) {
	fontName := "DejaVuSans"
	desc := fmt.Sprintf("font:%s, points:%d, position:bl, offset:%.1f %.1f, scalefactor:1 abs, color:0 0 0, opacity:1, rotation:0",
		fontName, fontSize, x, y)
	wm, err := api.TextWatermark(text, desc, true, false, types.POINTS)
	if err != nil {
		return
	}
	wmMap[pageNum] = append(wmMap[pageNum], wm)
}

// buildBlankPagePDF creates a minimal single-page blank PDF (US Letter size)
func buildBlankPagePDF() []byte {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	off1 := pdf.Len()
	pdf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := pdf.Len()
	pdf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	off3 := pdf.Len()
	pdf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n")
	off4 := pdf.Len()
	pdf.WriteString("4 0 obj\n<< /Length 0 >>\nstream\n\nendstream\nendobj\n")
	xref := pdf.Len()
	pdf.WriteString("xref\n0 5\n")
	pdf.WriteString("0000000000 65535 f \n")
	pdf.WriteString(fmt.Sprintf("%010d 00000 n \n", off1))
	pdf.WriteString(fmt.Sprintf("%010d 00000 n \n", off2))
	pdf.WriteString(fmt.Sprintf("%010d 00000 n \n", off3))
	pdf.WriteString(fmt.Sprintf("%010d 00000 n \n", off4))
	pdf.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n")
	pdf.WriteString(fmt.Sprintf("%d\n", xref))
	pdf.WriteString("%%EOF\n")
	return pdf.Bytes()
}

// createWhitePNG creates a white PNG image of the specified dimensions in points.
// At 72 DPI, 1 pixel = 1 point, so the PNG dimensions map directly to PDF points.
func createWhitePNG(widthPt, heightPt int) []byte {
	if widthPt < 1 {
		widthPt = 1
	}
	if heightPt < 1 {
		heightPt = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, widthPt, heightPt))
	// Fill with white (RGBA default is transparent black, so we must set each pixel)
	for y := 0; y < heightPt; y++ {
		for x := 0; x < widthPt; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

