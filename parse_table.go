package pdfx

import (
	"bytes"
	"errors"
	"strconv"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

func parseHTMLTable(htmlStr string, defaultFontSize float64) ([]TableRow, int, TableHTMLMeta, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, 0, TableHTMLMeta{}, err
	}
	tableNode := findFirstElement(doc, "table")
	if tableNode == nil {
		return nil, 0, TableHTMLMeta{}, errors.New("html table not found")
	}

	var rows []TableRow
	headerRows := 0
	meta := TableHTMLMeta{
		Width:       parseWidth(tableNode),
		CellPadding: parseCellPadding(tableNode),
		BorderWidth: parseHTMLBorderWidth(tableNode),
	}
	meta.BorderColor, meta.BorderAlpha = parseHTMLBorderColor(tableNode)

	tableFontFamily := parseFontFamily(tableNode)
	tableFontSize := parseFontSize(tableNode)
	if tableFontSize <= 0 {
		tableFontSize = defaultFontSize
	}
	tableFontStyle := ""
	if s, ok := parseFontWeightStyle(tableNode); ok {
		tableFontStyle = s
	}
	tableAlign, tableAlignSet := parseAlignAttrOrStyle(tableNode)
	tableVAlign, tableVAlignSet := parseVAlignAttrOrStyle(tableNode)
	tableNoWrap := parseNoWrap(tableNode)
	tableLineHeight, tableLineHeightSet := parseLineHeightSpec(tableNode, tableFontSize)

	var walk func(n *html.Node, inThead bool)
	walk = func(n *html.Node, inThead bool) {
		if n.Type == html.ElementNode && n.Data == "thead" {
			inThead = true
		}
		if n.Type == html.ElementNode && n.Data == "tr" {
			rowIsHeader := inThead
			rowFontFamily := parseFontFamily(n)
			if rowFontFamily == "" {
				rowFontFamily = tableFontFamily
			}
			rowFontSize := parseFontSize(n)
			if rowFontSize <= 0 {
				rowFontSize = tableFontSize
			}
			rowFontStyle := tableFontStyle
			if s, ok := parseFontWeightStyle(n); ok {
				rowFontStyle = s
			}
			rowAlign, rowAlignSet := parseAlignAttrOrStyle(n)
			rowVAlign, rowVAlignSet := parseVAlignAttrOrStyle(n)
			if !rowAlignSet && tableAlignSet {
				rowAlign = tableAlign
			}
			if !rowVAlignSet && tableVAlignSet {
				rowVAlign = tableVAlign
			}
			rowNoWrap := parseNoWrap(n) || tableNoWrap
			rowMinHeight, rowMinHeightSet := parseHeightSpec(n)
			rowLineHeight, rowLineHeightSet := parseLineHeightSpec(n, rowFontSize)
			if !rowLineHeightSet && tableLineHeightSet {
				rowLineHeight = tableLineHeight
				rowLineHeightSet = true
			}

			r := TableRow{MinHeight: rowMinHeight, MinHeightSet: rowMinHeightSet}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode {
					continue
				}
				if c.Data != "td" && c.Data != "th" {
					continue
				}

				isHeader := c.Data == "th" || rowIsHeader
				align := parseAlignWithFallback(c, rowAlign, isHeader)
				valign := parseVAlignWithFallback(c, rowVAlign)
				fontFamily := parseFontFamily(c)
				if fontFamily == "" {
					fontFamily = rowFontFamily
				}
				fontSize := parseFontSize(c)
				if fontSize <= 0 {
					fontSize = rowFontSize
				}
				if fontSize <= 0 {
					fontSize = tableFontSize
				}
				fontStyle := rowFontStyle
				if s, ok := parseFontWeightStyle(c); ok {
					fontStyle = s
				}

				cellPadding, cellPaddingSet := parseCellPaddingBox(c)
				if !cellPaddingSet && meta.CellPadding > 0 {
					cellPadding = CellPadding{Left: meta.CellPadding, Top: meta.CellPadding, Right: meta.CellPadding, Bottom: meta.CellPadding}
					cellPaddingSet = true
				}
				if !cellPaddingSet {
					if p, ok := parseInnerPaddingBox(c); ok {
						cellPadding = p
						cellPaddingSet = true
					}
				}

				cellWidth, cellAutoWidth := parseWidthSpec(c)
				if widthIsAuto(c) {
					cellAutoWidth = true
				}
				cellNoWrap := parseNoWrap(c) || rowNoWrap
				cellMinHeight, cellMinHeightSet := parseHeightSpec(c)
				cellLineHeight, cellLineHeightSet := parseLineHeightSpec(c, fontSize)
				if !cellLineHeightSet && rowLineHeightSet {
					cellLineHeight = rowLineHeight
					cellLineHeightSet = true
				}

				cell := TableCell{
					Text:          collapseSpace(extractText(c)),
					ColSpan:       parseSpanAttr(c, "colspan"),
					RowSpan:       parseSpanAttr(c, "rowspan"),
					Align:         align,
					VAlign:        valign,
					IsHeader:      isHeader,
					FontFamily:    fontFamily,
					FontSize:      fontSize,
					FontStyle:     fontStyle,
					Width:         cellWidth,
					AutoWidth:     cellAutoWidth,
					NoWrap:        cellNoWrap,
					LineHeight:    cellLineHeight,
					LineHeightSet: cellLineHeightSet,
					MinHeight:     cellMinHeight,
					MinHeightSet:  cellMinHeightSet,
					Padding:       cellPadding,
					PaddingSet:    cellPaddingSet,
				}
				r.Cells = append(r.Cells, cell)
			}
			if len(r.Cells) > 0 {
				rows = append(rows, r)
				if inThead {
					headerRows++
				}
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inThead)
		}
	}
	walk(tableNode, false)

	return rows, headerRows, meta, nil
}

func parseFontWeightStyle(n *html.Node) (string, bool) {
	v := strings.TrimSpace(getAttr(n, "font-weight"))
	if v == "" {
		v = strings.TrimSpace(parseStyleValue(n, "font-weight"))
	}
	if v == "" {
		return "", false
	}
	if isBoldFontWeight(v) {
		return "B", true
	}
	return "", true
}

func parseFontFamily(n *html.Node) string {
	if n == nil {
		return ""
	}
	if v := strings.TrimSpace(getAttr(n, "font")); v != "" {
		return strings.TrimSpace(parseFirstFontFamily(v))
	}
	if v := strings.TrimSpace(parseStyleValue(n, "font-family")); v != "" {
		return strings.TrimSpace(parseFirstFontFamily(v))
	}
	return ""
}

func parseInnerPaddingBox(n *html.Node) (CellPadding, bool) {
	var walk func(x *html.Node) (CellPadding, bool)
	walk = func(x *html.Node) (CellPadding, bool) {
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			if p, ok := parseCellPaddingBox(c); ok {
				if p.Left > 0 || p.Top > 0 || p.Right > 0 || p.Bottom > 0 {
					return p, true
				}
			}
			if p, ok := walk(c); ok {
				return p, true
			}
		}
		return CellPadding{}, false
	}
	return walk(n)
}

func findFirstElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if r := findFirstElement(c, tag); r != nil {
			return r
		}
	}
	return nil
}

func extractText(n *html.Node) string {
	var b strings.Builder
	var walk func(x *html.Node, hidden bool)
	walk = func(x *html.Node, hidden bool) {
		if hidden || nodeIsHidden(x) {
			return
		}
		switch x.Type {
		case html.TextNode:
			b.WriteString(x.Data)
		case html.ElementNode:
			if x.Data == "br" {
				b.WriteString("\n")
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c, false)
		}
	}
	walk(n, false)
	return b.String()
}

func nodeIsHidden(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if v := strings.ToLower(strings.TrimSpace(getAttr(n, "hidden"))); v != "" {
		return true
	}
	if v := strings.ToLower(strings.TrimSpace(getAttr(n, "aria-hidden"))); v == "true" {
		return true
	}
	if v := strings.ToLower(strings.TrimSpace(parseStyleValue(n, "display"))); v == "none" {
		return true
	}
	if v := strings.ToLower(strings.TrimSpace(parseStyleValue(n, "visibility"))); v == "hidden" {
		return true
	}
	return false
}

func collapseSpace(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.Join(strings.Fields(lines[i]), " ")
	}
	out := strings.TrimSpace(strings.Join(lines, "\n"))
	return out
}

func parseSpanAttr(n *html.Node, name string) int {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			v, err := strconv.Atoi(strings.TrimSpace(a.Val))
			if err == nil && v > 0 {
				return v
			}
		}
	}
	return 1
}

func parseAlignAttrOrStyle(n *html.Node) (Align, bool) {
	if v := getAttr(n, "align"); v != "" {
		return parseAlignValue(v, false), true
	}
	if v := parseStyleValue(n, "text-align"); v != "" {
		return parseAlignValue(v, false), true
	}
	return AlignLeft, false
}

func parseAlignWithFallback(n *html.Node, fallback Align, headerDefaultCenter bool) Align {
	if v := getAttr(n, "align"); v != "" {
		return parseAlignValue(v, headerDefaultCenter)
	}
	if v := parseStyleValue(n, "text-align"); v != "" {
		return parseAlignValue(v, headerDefaultCenter)
	}
	if headerDefaultCenter {
		return AlignCenter
	}
	return fallback
}

func parseAlignValue(v string, headerDefaultCenter bool) Align {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "center", "middle":
		return AlignCenter
	case "right":
		return AlignRight
	case "left":
		return AlignLeft
	default:
		if headerDefaultCenter {
			return AlignCenter
		}
		return AlignLeft
	}
}

func parseVAlignAttrOrStyle(n *html.Node) (VAlign, bool) {
	if v := getAttr(n, "valign"); v != "" {
		return parseVAlignValue(v), true
	}
	if v := parseStyleValue(n, "vertical-align"); v != "" {
		return parseVAlignValue(v), true
	}
	return VAlignMiddle, false
}

func parseVAlignWithFallback(n *html.Node, fallback VAlign) VAlign {
	if v := getAttr(n, "valign"); v != "" {
		return parseVAlignValue(v)
	}
	if v := parseStyleValue(n, "vertical-align"); v != "" {
		return parseVAlignValue(v)
	}
	return fallback
}

func parseVAlignValue(v string) VAlign {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "top":
		return VAlignTop
	case "bottom":
		return VAlignBottom
	case "middle", "center":
		return VAlignMiddle
	default:
		return VAlignMiddle
	}
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if strings.EqualFold(n.Attr[i].Key, key) {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func parseStyleValue(n *html.Node, key string) string {
	style := getAttr(n, "style")
	if style == "" {
		return ""
	}
	parts := strings.Split(style, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := strings.TrimSpace(kv[1])
		if k == strings.ToLower(key) {
			return v
		}
	}
	return ""
}

func parseWidth(n *html.Node) float64 {
	if v := strings.TrimSpace(getAttr(n, "width")); v != "" {
		if w := parseCSSLengthToMM(v); w > 0 {
			return w
		}
	}
	if v := strings.TrimSpace(parseStyleValue(n, "width")); v != "" {
		if w := parseCSSLengthToMM(v); w > 0 {
			return w
		}
	}
	return 0
}

func parseWidthSpec(n *html.Node) (float64, bool) {
	if v := strings.TrimSpace(getAttr(n, "width")); v != "" {
		if w := parseCSSLengthToMM(v); w > 0 {
			return w, false
		}
		if strings.ToLower(v) == "auto" {
			return 0, true
		}
	}
	if v := strings.TrimSpace(parseStyleValue(n, "width")); v != "" {
		if w := parseCSSLengthToMM(v); w > 0 {
			return w, false
		}
		if strings.ToLower(v) == "auto" {
			return 0, true
		}
	}
	return 0, false
}

func widthIsAuto(n *html.Node) bool {
	v := strings.ToLower(strings.TrimSpace(getAttr(n, "width")))
	if v != "" {
		v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
		return v == "auto"
	}
	v = strings.ToLower(strings.TrimSpace(parseStyleValue(n, "width")))
	v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
	return v == "auto"
}

func parseFontSize(n *html.Node) float64 {
	if v := strings.TrimSpace(getAttr(n, "font-size")); v != "" {
		return parseFontSizeToPoints(v)
	}
	if v := strings.TrimSpace(parseStyleValue(n, "font-size")); v != "" {
		return parseFontSizeToPoints(v)
	}
	return 0
}

func parseFontSizeToPoints(v string) float64 {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimSuffix(v, "!important")
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if strings.HasSuffix(v, "px") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "px")), 64)
		if err == nil && num > 0 {
			return num * 0.75
		}
	}
	if strings.HasSuffix(v, "pt") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "pt")), 64)
		if err == nil && num > 0 {
			return num
		}
	}
	if num, err := strconv.ParseFloat(v, 64); err == nil && num > 0 {
		return num
	}
	return 0
}

func parseNoWrap(n *html.Node) bool {
	if v := strings.ToLower(strings.TrimSpace(getAttr(n, "nowrap"))); v != "" {
		return true
	}
	if v := strings.ToLower(strings.TrimSpace(parseStyleValue(n, "white-space"))); v != "" {
		v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
		return v == "nowrap"
	}
	return false
}

func isPureNumber(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			continue
		}
		return false
	}
	return true
}

func parseCellPadding(n *html.Node) float64 {
	if v := strings.TrimSpace(getAttr(n, "cellpadding")); v != "" {
		if isPureNumber(v) {
			if w := parseCSSLengthToMM(v + "px"); w > 0 {
				return w
			}
		} else {
			if w := parseCSSLengthToMM(v); w > 0 {
				return w
			}
		}
	}
	if v := strings.TrimSpace(parseStyleValue(n, "padding")); v != "" {
		pt, pr, pb, pl, err := parsePaddingShorthandToMM(v)
		if err == nil {
			if pt == pr && pr == pb && pb == pl {
				return pt
			}
		}
	}
	return 0
}

func parseCellPaddingBox(n *html.Node) (CellPadding, bool) {
	if v := strings.TrimSpace(getAttr(n, "cellpadding")); v != "" {
		if isPureNumber(v) {
			if w := parseCSSLengthToMM(v + "px"); w > 0 {
				return CellPadding{Left: w, Top: w, Right: w, Bottom: w}, true
			}
		} else {
			if w := parseCSSLengthToMM(v); w > 0 {
				return CellPadding{Left: w, Top: w, Right: w, Bottom: w}, true
			}
		}
	}
	if v := strings.TrimSpace(getAttr(n, "padding")); v != "" {
		pt, pr, pb, pl, err := parsePaddingShorthandToMM(v)
		if err == nil {
			return CellPadding{Left: pl, Top: pt, Right: pr, Bottom: pb}, true
		}
	}
	if v := strings.TrimSpace(parseStyleValue(n, "padding")); v != "" {
		pt, pr, pb, pl, err := parsePaddingShorthandToMM(v)
		if err == nil {
			return CellPadding{Left: pl, Top: pt, Right: pr, Bottom: pb}, true
		}
	}

	pt := parseCSSLengthToMM(parseStyleValue(n, "padding-top"))
	pr := parseCSSLengthToMM(parseStyleValue(n, "padding-right"))
	pb := parseCSSLengthToMM(parseStyleValue(n, "padding-bottom"))
	pl := parseCSSLengthToMM(parseStyleValue(n, "padding-left"))
	if pt > 0 || pr > 0 || pb > 0 || pl > 0 {
		return CellPadding{Left: pl, Top: pt, Right: pr, Bottom: pb}, true
	}
	return CellPadding{}, false
}

func parseHTMLBorderWidth(n *html.Node) float64 {
	if v := strings.TrimSpace(getAttr(n, "border")); v != "" {
		if isPureNumber(v) {
			if w := parseCSSLengthToMM(v + "px"); w > 0 {
				return w
			}
		} else {
			if w := parseCSSLengthToMM(v); w > 0 {
				return w
			}
		}
	}
	if v := strings.TrimSpace(parseStyleValue(n, "border-width")); v != "" {
		if w := parseCSSLengthToMM(v); w > 0 {
			return w
		}
	}
	return 0
}

// parseHTMLBorderColor 取 <table> 上显式声明的边框颜色，支持：
//
//	border="1pt #333333"（简写）、border-color="#333333"、
//	style="border-color:#333333"、style="border:1pt solid #333333"
//
// 没有声明时返回零值，由 applyDefaultsBorder 补成黑色、alpha 补成 1。
func parseHTMLBorderColor(n *html.Node) ([3]uint8, float64) {
	for _, src := range []string{
		strings.TrimSpace(getAttr(n, "border")),
		strings.TrimSpace(parseStyleValue(n, "border")),
	} {
		if src == "" {
			continue
		}
		if c, a, ok := colorInBorderShorthand(src); ok {
			return c, a
		}
	}
	for _, v := range []string{
		strings.TrimSpace(getAttr(n, "border-color")),
		strings.TrimSpace(parseStyleValue(n, "border-color")),
	} {
		if v == "" {
			continue
		}
		if c, a, ok := parseColorSpecAlpha(v); ok {
			return c, a
		}
	}
	return [3]uint8{}, 0
}

// colorInBorderShorthand 从 border 简写里挑出颜色词（含 alpha）。
//
// 不能直接拿 parseBorderValue 的返回值：它在没写颜色时会补一个黑色，
// 那样 border="1pt" + border-color="#333333" 会被顶回黑色。
func colorInBorderShorthand(src string) ([3]uint8, float64, bool) {
	src = strings.ToLower(strings.TrimSpace(src))
	src = strings.TrimSpace(strings.TrimSuffix(src, "!important"))
	if src == "" || strings.Contains(src, "none") {
		return [3]uint8{}, 0, false
	}
	for _, f := range strings.Fields(src) {
		f = strings.TrimSpace(strings.TrimSuffix(f, "!important"))
		if f == "" {
			continue
		}
		if c, a, ok := parseColorSpecAlpha(f); ok {
			return c, a, true
		}
	}
	return [3]uint8{}, 0, false
}

func parseCSSLengthToMM(v string) float64 {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
	if v == "" {
		return 0
	}
	if strings.HasSuffix(v, "mm") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "mm")), 64)
		if err == nil && num > 0 {
			return num
		}
		return 0
	}
	if strings.HasSuffix(v, "cm") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "cm")), 64)
		if err == nil && num > 0 {
			return num * 10
		}
		return 0
	}
	if strings.HasSuffix(v, "in") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "in")), 64)
		if err == nil && num > 0 {
			return num * 25.4
		}
		return 0
	}
	if strings.HasSuffix(v, "px") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "px")), 64)
		if err == nil && num > 0 {
			return num * 0.2645833333
		}
		return 0
	}
	if strings.HasSuffix(v, "pt") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "pt")), 64)
		if err == nil && num > 0 {
			return num * 25.4 / 72
		}
		return 0
	}
	if strings.HasSuffix(v, "pc") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "pc")), 64)
		if err == nil && num > 0 {
			return num * 12 * 25.4 / 72
		}
		return 0
	}
	if strings.HasSuffix(v, "em") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "em")), 64)
		if err == nil && num > 0 {
			return gopdf.PointsToUnits(gopdf.UnitMM, 9*num)
		}
		return 0
	}
	if strings.HasSuffix(v, "rem") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "rem")), 64)
		if err == nil && num > 0 {
			return gopdf.PointsToUnits(gopdf.UnitMM, 9*num)
		}
		return 0
	}
	if num, err := strconv.ParseFloat(v, 64); err == nil && num > 0 {
		return num
	}
	return 0
}

func parsePaddingShorthandToMM(s string) (top, right, bottom, left float64, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, 0, 0, nil
	}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0, 0, 0, 0, nil
	}
	vals := make([]float64, 0, 4)
	for _, p := range parts {
		v := parseCSSLengthToMM(p)
		if v < 0 {
			return 0, 0, 0, 0, errors.New("invalid padding")
		}
		vals = append(vals, v)
	}
	switch len(vals) {
	case 1:
		top, right, bottom, left = vals[0], vals[0], vals[0], vals[0]
	case 2:
		top, bottom = vals[0], vals[0]
		right, left = vals[1], vals[1]
	case 3:
		top, right, left = vals[0], vals[1], vals[1]
		bottom = vals[2]
	default:
		top, right, bottom, left = vals[0], vals[1], vals[2], vals[3]
	}
	return top, right, bottom, left, nil
}

func renderOuterHTML(n *html.Node) (string, error) {
	var b bytes.Buffer
	if err := html.Render(&b, n); err != nil {
		return "", err
	}
	return b.String(), nil
}
