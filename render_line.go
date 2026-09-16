package pdfx

import (
	"errors"
	"strconv"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

func DrawLineFromNode(pdf *gopdf.GoPdf, n *html.Node) error {
	if pdf == nil {
		return errors.New("line: nil GoPdf")
	}
	if n == nil || n.Type != html.ElementNode || !strings.EqualFold(n.Data, "line") {
		return nil
	}

	left, _, right, _ := pdf.Margins()
	pageW, ok := getGoPdfPageWidthInUnits(pdf)
	if !ok || pageW <= 0 {
		pageW = left + right + 100
	}
	contentW := pageW - left - right
	if contentW < 0 {
		contentW = 0
	}

	y0 := pdf.GetY()

	hStr := strings.TrimSpace(getAttr(n, "height"))
	if hStr == "" {
		hStr = strings.TrimSpace(parseStyleValue(n, "height"))
	}
	thickness := 0.0
	if hStr == "" {
		thickness = parseCSSLengthToMM("1px")
	} else if isPureNumber(hStr) {
		thickness = parseCSSLengthToMM(hStr + "px")
	} else {
		thickness = parseCSSLengthToMM(hStr)
	}
	if thickness <= 0 {
		thickness = parseCSSLengthToMM("1px")
	}

	wStr := strings.TrimSpace(getAttr(n, "width"))
	if wStr == "" {
		wStr = strings.TrimSpace(parseStyleValue(n, "width"))
	}
	lineW := 0.0
	if strings.HasSuffix(wStr, "%") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(wStr, "%")), 64)
		if err == nil && num > 0 {
			lineW = contentW * num / 100.0
		}
	} else if wStr != "" {
		if isPureNumber(wStr) {
			lineW = parseCSSLengthToMM(wStr + "px")
		} else {
			lineW = parseCSSLengthToMM(wStr)
		}
	}
	if lineW <= 0 {
		lineW = contentW
	}
	if lineW < 0 {
		lineW = 0
	}

	x0 := left
	marginStr := strings.ToLower(strings.TrimSpace(parseStyleValue(n, "margin")))
	if strings.Contains(marginStr, "auto") {
		if contentW > lineW {
			x0 = left + (contentW-lineW)/2.0
		}
	}
	x1 := x0 + lineW

	r, g, b := uint8(0), uint8(0), uint8(0)
	if cStr := strings.TrimSpace(getAttr(n, "color")); cStr != "" {
		if rr, gg, bb, ok := parseHexColor(cStr); ok {
			r, g, b = rr, gg, bb
		}
	}

	lineType := strings.ToLower(strings.TrimSpace(getAttr(n, "type")))
	switch lineType {
	case "dashed", "dotted", "solid":
		pdf.SetLineType(lineType)
	default:
		pdf.SetLineType("solid")
	}

	text := strings.TrimSpace(getAttr(n, "text"))
	if text == "" {
		text = strings.TrimSpace(extractText(n))
	}

	fontSizePt, ok := getGoPdfCurrentFontSizePoints(pdf)
	lineHeight := 0.0
	if ok && fontSizePt > 0 {
		lineHeight = pdf.PointsToUnits(fontSizePt * 1.25)
	}
	if lineHeight <= 0 {
		lineHeight = pdf.PointsToUnits(9 * 1.25)
	}
	blockH := thickness
	if lineHeight > blockH {
		blockH = lineHeight
	}

	pdf.SetLineWidth(thickness)
	pdf.SetStrokeColor(r, g, b)

	yLine := y0 + blockH/2.0
	if text == "" {
		pdf.Line(x0, yLine, x1, yLine)
		pdf.SetLineType("solid")
		pdf.SetY(y0 + blockH)
		return nil
	}

	padStr := strings.TrimSpace(getAttr(n, "text-padding"))
	if padStr == "" {
		padStr = strings.TrimSpace(parseStyleValue(n, "text-padding"))
	}
	pt, pr, pb, pl := 0.0, 0.0, 0.0, 0.0
	if padStr != "" {
		tpt, tpr, tpb, tpl, err := parsePaddingShorthandToMM(padStr)
		if err == nil {
			pt, pr, pb, pl = tpt, tpr, tpb, tpl
		}
	}

	textW, err := pdf.MeasureTextWidth(text)
	if err != nil {
		textW = 0
	}
	gapW := textW + pl + pr
	if gapW >= lineW || gapW <= 0 {
		pdf.Line(x0, yLine, x1, yLine)
		yText := y0 + (blockH-lineHeight)/2.0
		pdf.SetXY(x0, yText)
		_ = pt
		_ = pb
		_ = pdf.CellWithOption(&gopdf.Rect{W: lineW, H: lineHeight}, text, gopdf.CellOption{Align: gopdf.Center | gopdf.Top})
		pdf.SetLineType("solid")
		pdf.SetY(y0 + blockH)
		return nil
	}

	alignStr := strings.ToLower(strings.TrimSpace(getAttr(n, "text-align")))
	if alignStr == "" {
		alignStr = strings.ToLower(strings.TrimSpace(parseStyleValue(n, "text-align")))
	}

	xGapStart := x0
	switch alignStr {
	case "right":
		xGapStart = x1 - gapW
	case "left":
		xGapStart = x0
	default:
		xGapStart = x0 + (lineW-gapW)/2.0
	}
	xGapEnd := xGapStart + gapW
	if xGapStart > x0 {
		pdf.Line(x0, yLine, xGapStart, yLine)
	}
	if xGapEnd < x1 {
		pdf.Line(xGapEnd, yLine, x1, yLine)
	}

	yText := y0 + (blockH-lineHeight)/2.0
	pdf.SetXY(xGapStart+pl, yText)
	_ = pt
	_ = pb
	_ = pdf.CellWithOption(&gopdf.Rect{W: textW + 0.01, H: lineHeight}, text, gopdf.CellOption{Align: gopdf.Left | gopdf.Top})

	pdf.SetLineType("solid")
	pdf.SetY(y0 + blockH)
	return nil
}
