package pdfx

import (
	"strconv"
	"strings"

	"github.com/signintech/gopdf"
)

type Row struct {
	Height            float64
	LineHeight        float64
	Padding           [4]float64
	Align             string
	VAlignSet         bool
	VAlign            VAlign
	FontFamily        string
	FontStyle         string
	FontSize          float64
	Cols              []Col
	FontWeightSet     bool
	FontBold          bool
	NoWrapSet         bool
	NoWrap            bool
	BorderSet         bool
	BorderWidth       float64
	BorderColor       [3]uint8
	BorderTopSet      bool
	BorderTopWidth    float64
	BorderTopColor    [3]uint8
	BorderRightSet    bool
	BorderRightWidth  float64
	BorderRightColor  [3]uint8
	BorderBottomSet   bool
	BorderBottomWidth float64
	BorderBottomColor [3]uint8
	BorderLeftSet     bool
	BorderLeftWidth   float64
	BorderLeftColor   [3]uint8
}

type Col struct {
	Row
	Width      string
	Text       string
	PaddingSet bool
	Box        *Box
}

type Box struct {
	Row
	Text       string
	PaddingSet bool
	Width      string
}

func isAutoWidthSpec(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
	return v == "auto"
}

func computeColWidths(cols []Col, totalWidth float64) ([]float64, error) {
	if len(cols) == 0 {
		return nil, nil
	}
	out := make([]float64, len(cols))
	var fixed float64
	var percentSum float64
	var percentIdx []int
	var autoIdx []int

	for i, c := range cols {
		w := strings.TrimSpace(c.Width)
		if w == "" {
			autoIdx = append(autoIdx, i)
			continue
		}
		if strings.HasSuffix(w, "%") {
			num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(w, "%")), 64)
			if err != nil || num < 0 {
				autoIdx = append(autoIdx, i)
				continue
			}
			percentSum += num
			percentIdx = append(percentIdx, i)
			out[i] = num
			continue
		}
		val := parseCSSLengthToMM(w)
		if val <= 0 {
			if num, err := strconv.ParseFloat(w, 64); err == nil && num > 0 {
				val = num
			}
		}
		if val <= 0 {
			autoIdx = append(autoIdx, i)
			continue
		}
		out[i] = val
		fixed += val
	}

	remain := totalWidth - fixed
	if remain < 0 {
		remain = 0
	}

	if len(percentIdx) > 0 {
		den := percentSum
		if den <= 0 {
			den = 100
		}
		for _, i := range percentIdx {
			pct := out[i]
			out[i] = remain * pct / den
		}
	}

	var used float64
	for i := range out {
		if i < len(cols) && (strings.TrimSpace(cols[i].Width) != "" && !strings.HasSuffix(strings.TrimSpace(cols[i].Width), "%")) {
			used += out[i]
		}
	}
	for _, i := range percentIdx {
		used += out[i]
	}
	remain2 := totalWidth - used
	if remain2 < 0 {
		remain2 = 0
	}
	if len(autoIdx) > 0 {
		each := remain2 / float64(len(autoIdx))
		for _, i := range autoIdx {
			out[i] = each
		}
	}

	return out, nil
}

func boxStyleFromInherited(inherited Row, box *Box) Row {
	s := inherited
	s.Padding = [4]float64{}
	s.BorderSet = false
	s.BorderWidth = 0
	s.BorderColor = [3]uint8{}
	s.BorderTopSet = false
	s.BorderTopWidth = 0
	s.BorderTopColor = [3]uint8{}
	s.BorderRightSet = false
	s.BorderRightWidth = 0
	s.BorderRightColor = [3]uint8{}
	s.BorderBottomSet = false
	s.BorderBottomWidth = 0
	s.BorderBottomColor = [3]uint8{}
	s.BorderLeftSet = false
	s.BorderLeftWidth = 0
	s.BorderLeftColor = [3]uint8{}
	if box == nil {
		return s
	}
	if box.Height > 0 {
		s.Height = box.Height
	}
	if box.LineHeight > 0 {
		s.LineHeight = box.LineHeight
	}
	if box.Align != "" {
		s.Align = box.Align
	}
	if box.VAlignSet {
		s.VAlignSet = true
		s.VAlign = box.VAlign
	}
	if box.FontFamily != "" {
		s.FontFamily = box.FontFamily
	}
	if box.FontSize > 0 {
		s.FontSize = box.FontSize
	}
	if box.FontStyle != "" {
		s.FontStyle = box.FontStyle
	}
	if box.FontWeightSet {
		s.FontWeightSet = true
		s.FontBold = box.FontBold
		if box.FontBold {
			s.FontStyle = "B"
		} else {
			s.FontStyle = ""
		}
	}
	if box.NoWrapSet {
		s.NoWrapSet = true
		s.NoWrap = box.NoWrap
	}
	if box.PaddingSet {
		s.Padding = box.Padding
	}
	if box.BorderSet {
		s.BorderSet = true
		s.BorderWidth = box.BorderWidth
		s.BorderColor = box.BorderColor
		s.BorderTopSet = false
		s.BorderRightSet = false
		s.BorderBottomSet = false
		s.BorderLeftSet = false
	}
	if box.BorderTopSet {
		s.BorderTopSet = true
		s.BorderTopWidth = box.BorderTopWidth
		s.BorderTopColor = box.BorderTopColor
	}
	if box.BorderRightSet {
		s.BorderRightSet = true
		s.BorderRightWidth = box.BorderRightWidth
		s.BorderRightColor = box.BorderRightColor
	}
	if box.BorderBottomSet {
		s.BorderBottomSet = true
		s.BorderBottomWidth = box.BorderBottomWidth
		s.BorderBottomColor = box.BorderBottomColor
	}
	if box.BorderLeftSet {
		s.BorderLeftSet = true
		s.BorderLeftWidth = box.BorderLeftWidth
		s.BorderLeftColor = box.BorderLeftColor
	}
	return s
}

func computeBoxLayout(pdf *gopdf.GoPdf, inherited Row, box *Box, availW float64) (style Row, lines []string, lineHeight float64, boxW float64, boxH float64, fakeBold bool, err error) {
	style = boxStyleFromInherited(inherited, box)
	if box == nil {
		return style, []string{""}, 0, 0, 0, false, nil
	}

	family := style.FontFamily
	fontStyle := style.FontStyle
	fontSize := style.FontSize
	if family == "" {
		family = "songti"
	}
	if fontSize <= 0 {
		fontSize = 9
	}
	if err := pdf.SetFont(family, fontStyle, fontSize); err != nil {
		if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fontStyle), "B") {
			if err2 := pdf.SetFont(family, "", fontSize); err2 != nil {
				return style, nil, 0, 0, 0, false, err2
			}
			fakeBold = true
			fontStyle = ""
			style.FontStyle = ""
		} else {
			return style, nil, 0, 0, 0, false, err
		}
	}

	lineHeight = style.LineHeight
	if lineHeight <= 0 {
		lineHeight = pdf.PointsToUnits(fontSize * 1.25)
	}

	padTop := style.Padding[0]
	padRight := style.Padding[1]
	padBot := style.Padding[2]
	padLeft := style.Padding[3]
	bwTop, bwRight, bwBottom, bwLeft := borderWidthsEffective(style)

	if w := parseBoxWidthToMM(box.Width, availW); w > 0 {
		boxW = w
		if availW > 0 && boxW > availW {
			boxW = availW
		}
	}

	contentAvailW := availW
	if boxW > 0 {
		contentAvailW = boxW
	}
	contentAvailW = contentAvailW - bwLeft - bwRight - padLeft - padRight
	if contentAvailW < 0 {
		contentAvailW = 0
	}

	txt := collapseSpace(box.Text)
	if style.NoWrap {
		lines = []string{txt}
		if boxW <= 0 {
			w, errW := pdf.MeasureTextWidth(txt)
			if errW != nil {
				return style, nil, 0, 0, 0, fakeBold, errW
			}
			boxW = w + padLeft + padRight + bwLeft + bwRight
			if availW > 0 && boxW > availW {
				boxW = availW
			}
		}
		boxH = padTop + padBot + lineHeight + bwTop + bwBottom
		if style.Height > 0 {
			boxH = style.Height
		}
		return style, lines, lineHeight, boxW, boxH, fakeBold, nil
	}

	lines, err = wrapText(pdf, txt, contentAvailW)
	if err != nil {
		return style, nil, 0, 0, 0, fakeBold, err
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	maxW := 0.0
	for _, ln := range lines {
		w, errW := pdf.MeasureTextWidth(ln)
		if errW != nil {
			return style, nil, 0, 0, 0, fakeBold, errW
		}
		if w > maxW {
			maxW = w
		}
	}
	if boxW <= 0 {
		boxW = maxW + padLeft + padRight + bwLeft + bwRight
		if availW > 0 && boxW > availW {
			boxW = availW
		}
	}
	boxH = padTop + padBot + float64(len(lines))*lineHeight + bwTop + bwBottom
	if style.Height > 0 {
		boxH = style.Height
	}
	return style, lines, lineHeight, boxW, boxH, fakeBold, nil
}

type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

type VAlign int

const (
	VAlignTop VAlign = iota
	VAlignMiddle
	VAlignBottom
)
