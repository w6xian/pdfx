package pdfx

import (
	"reflect"

	"github.com/signintech/gopdf"
)

type PageStyle string

var (
	A3                PageStyle = "A3"
	A3Landscape       PageStyle = "A3.landscape"
	A4                PageStyle = "A4"
	A4Landscape       PageStyle = "A4.landscape"
	L3One             PageStyle = "L3-1"
	L3Two             PageStyle = "L3-2"
	L3Thr             PageStyle = "L3-3"
	A5                PageStyle = "A5"
	A5Landscape       PageStyle = "A5.landscape"
	Letter            PageStyle = "letter"
	LetterLandscape   PageStyle = "letter.landscape "
	Legal             PageStyle = "legal"
	LegalLandscape    PageStyle = "legal.landscape"
	Document          PageStyle = "document"
	DocumentLandscape PageStyle = "document.landscape"
	Ratio                       = 0.0393700787
)

// 页面尺寸速查表（单位 mm）：
//
//	1mm == 0.0393700787
//	A3                  { width: 297mm; height: 419mm }
//	A3.landscape        { width: 420mm; height: 296mm }
//	A4                  { width: 210mm; height: 296mm }
//	A4.landscape        { width: 297mm; height: 209mm }
//	L3-1                { width: 214mm; height: 93mm }
//	L3-2                { width: 214mm; height: 140mm }
//	L3-3                { width: 214mm; height: 280mm }
//	A5                  { width: 148mm; height: 209mm }
//	A5.landscape        { width: 210mm; height: 147mm }
//	letter              { width: 216mm; height: 279mm }
//	letter.landscape    { width: 280mm; height: 215mm }
//	legal               { width: 216mm; height: 356mm }
//	legal.landscape     { width: 357mm; height: 215mm }
//	document            { width: 120mm; height: 240mm }
//	document.landscape  { width: 240mm; height: 120mm }
type Paper struct {
	Width  float64
	Height float64
}

func GetPageStyle(name string) PageStyle {
	switch name {
	case "A3":
		return A3
	case "A3.landscape":
		return A3Landscape
	case "A4":
		return A4
	case "A4.landscape":
		return A4Landscape
	case "L3-1":
		return L3One
	case "L3-2":
		return L3Two
	case "L3-3":
		return L3Thr
	case "A5":
		return A5
	case "A5.landscape":
		return A5Landscape
	case "letter":
		return Letter
	case "letter.landscape":
		return LetterLandscape
	case "legal":
		return Legal
	case "legal.landscape":
		return LegalLandscape
	case "document":
		return Document
	case "document.landscape":
		return DocumentLandscape
	default:
		return A4
	}
}

// 页面尺寸速查表（单位 mm）：
//
//	1mm == 0.0393700787
//	A3                  { width: 297mm; height: 419mm }
//	A3.landscape        { width: 420mm; height: 296mm }
//	A4                  { width: 210mm; height: 296mm }
//	A4.landscape        { width: 297mm; height: 209mm }
//	L3-1                { width: 214mm; height: 93mm }
//	L3-2                { width: 214mm; height: 140mm }
//	L3-3                { width: 214mm; height: 280mm }
//	A5                  { width: 148mm; height: 209mm }
//	A5.landscape        { width: 210mm; height: 147mm }
//	letter              { width: 216mm; height: 279mm }
//	letter.landscape    { width: 280mm; height: 215mm }
//	legal               { width: 216mm; height: 356mm }
//	legal.landscape     { width: 357mm; height: 215mm }
//	document            { width: 120mm; height: 240mm }
//	document.landscape  { width: 240mm; height: 120mm }

func (p *Paper) FixWithPageStyle(name PageStyle) *Paper {
	switch name {
	case A3:
		p.Width = 297
		p.Height = 419
	case A3Landscape:
		p.Width = 420
		p.Height = 296
	case A4:
		p.Width = 210
		p.Height = 296
	case A4Landscape:
		p.Width = 297
		p.Height = 209
	case L3One:
		p.Width = 214
		p.Height = 93
	case L3Two:
		p.Width = 214
		p.Height = 140
	case L3Thr:
		p.Width = 214
		p.Height = 280
	case A5:
		p.Width = 148
		p.Height = 209
	case A5Landscape:
		p.Width = 210
		p.Height = 147
	case Letter:
		p.Width = 216
		p.Height = 279
	case LetterLandscape:
		p.Width = 280
		p.Height = 215
	case Legal:
		p.Width = 216
		p.Height = 356
	case LegalLandscape:
		p.Width = 357
		p.Height = 215
	case Document:
		p.Width = 120
		p.Height = 240
	case DocumentLandscape:
		p.Width = 240
		p.Height = 120
	}
	return p
}

func NewPager(style PageStyle) *Paper {
	p := &Paper{}
	return p.FixWithPageStyle(style)
}

type Padding struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
}

func NewPadding(p1 float64, pn ...float64) *Padding {
	p := &Padding{Left: 0, Top: 0, Right: 0, Bottom: 0}
	l := len(pn)
	if l == 0 {
		p.Top = p1
		p.Bottom = p1
		p.Left = p1
		p.Right = p1
	} else if l == 1 {
		p.Top = p1
		p.Bottom = p1
		p.Left = pn[0]
		p.Right = pn[0]
	} else if l == 2 {
		p.Top = p1
		p.Left = pn[0]
		p.Right = pn[1]
		p.Bottom = pn[0]
	} else {
		p.Top = p1
		p.Right = pn[0]
		p.Bottom = pn[1]
		p.Left = pn[2]
	}
	return p
}
func getGoPdfPageWidthInUnits(pdf *gopdf.GoPdf) (float64, bool) {
	rv := reflect.ValueOf(pdf)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return 0, false
	}
	rv = rv.Elem()
	curr := rv.FieldByName("curr")
	if !curr.IsValid() {
		return 0, false
	}
	pageSize := curr.FieldByName("pageSize")
	if !pageSize.IsValid() || pageSize.Kind() != reflect.Ptr || pageSize.IsNil() {
		return 0, false
	}
	rect := pageSize.Elem()
	w := rect.FieldByName("W")
	if !w.IsValid() {
		return 0, false
	}
	val := w.Float()
	if val <= 0 {
		return 0, false
	}
	config := rv.FieldByName("config")
	unitVal := config.FieldByName("Unit")
	if unitVal.IsValid() && unitVal.Int() == int64(gopdf.UnitMM) && val > 400 {
		return pdf.PointsToUnits(val), true
	}
	return val, true
}

func getGoPdfPageHeightInUnits(pdf *gopdf.GoPdf) (float64, bool) {
	rv := reflect.ValueOf(pdf)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return 0, false
	}
	rv = rv.Elem()
	curr := rv.FieldByName("curr")
	if !curr.IsValid() {
		return 0, false
	}
	pageSize := curr.FieldByName("pageSize")
	if !pageSize.IsValid() || pageSize.Kind() != reflect.Ptr || pageSize.IsNil() {
		return 0, false
	}
	rect := pageSize.Elem()
	h := rect.FieldByName("H")
	if !h.IsValid() {
		return 0, false
	}
	val := h.Float()
	if val <= 0 {
		return 0, false
	}
	config := rv.FieldByName("config")
	unitVal := config.FieldByName("Unit")
	if unitVal.IsValid() && unitVal.Int() == int64(gopdf.UnitMM) && val > 400 {
		return pdf.PointsToUnits(val), true
	}
	return val, true
}
