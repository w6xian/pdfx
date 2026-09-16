package pdfx

import (
	"strings"

	"golang.org/x/net/html"
)

func parseRowNode(n *html.Node) Row {
	r := parseRowStyle(n)

	var cols []Col
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if !(strings.EqualFold(c.Data, "col") || strings.EqualFold(c.Data, "x-col")) {
			continue
		}
		colStyle := parseRowStyle(c)
		col := Col{
			Row:   colStyle,
			Width: strings.TrimSpace(getAttr(c, "width")),
			Text:  collapseSpace(extractText(c)),
		}
		if col.Width == "" {
			col.Width = strings.TrimSpace(parseStyleValue(c, "width"))
		}
		col.PaddingSet = colPaddingSpecified(c)

		if boxNode := onlyElementChild(c, "box"); boxNode != nil {
			boxStyle := parseRowStyle(boxNode)
			b := &Box{
				Row:  boxStyle,
				Text: collapseSpace(extractText(boxNode)),
			}
			b.Width = strings.TrimSpace(getAttr(boxNode, "width"))
			if b.Width == "" {
				b.Width = strings.TrimSpace(parseStyleValue(boxNode, "width"))
			}
			b.PaddingSet = colPaddingSpecified(boxNode)
			col.Box = b
		}
		cols = append(cols, col)
	}
	r.Cols = cols

	return r
}

func readPdfAttributesHTML(n *html.Node) (width float64, height float64, paper string, padding string) {
	width = parseCSSLengthToMM(getAttr(n, "width"))
	height = parseCSSLengthToMM(getAttr(n, "height"))
	paper = getAttr(n, "paper")
	padding = getAttr(n, "padding")
	return
}

func normalizeRowCustomTags(s string) string {
	if strings.IndexByte(s, '<') < 0 {
		return s
	}

	isTerm := func(b byte) bool {
		switch b {
		case ' ', '\t', '\r', '\n', '>', '/':
			return true
		default:
			return false
		}
	}

	var out strings.Builder
	out.Grow(len(s) + 16)
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			out.WriteByte(s[i])
			continue
		}

		if i+4 < len(s) &&
			(s[i+1] == 'c' || s[i+1] == 'C') &&
			(s[i+2] == 'o' || s[i+2] == 'O') &&
			(s[i+3] == 'l' || s[i+3] == 'L') &&
			isTerm(s[i+4]) {
			out.WriteString("<x-col")
			i += 3
			continue
		}
		if i+5 < len(s) &&
			s[i+1] == '/' &&
			(s[i+2] == 'c' || s[i+2] == 'C') &&
			(s[i+3] == 'o' || s[i+3] == 'O') &&
			(s[i+4] == 'l' || s[i+4] == 'L') &&
			isTerm(s[i+5]) {
			out.WriteString("</x-col")
			i += 4
			continue
		}

		out.WriteByte(s[i])
	}
	return out.String()
}

func colPaddingSpecified(n *html.Node) bool {
	if strings.TrimSpace(getAttr(n, "padding")) != "" {
		return true
	}
	if strings.TrimSpace(parseStyleValue(n, "padding")) != "" {
		return true
	}
	if strings.TrimSpace(parseStyleValue(n, "padding-left")) != "" {
		return true
	}
	if strings.TrimSpace(parseStyleValue(n, "padding-right")) != "" {
		return true
	}
	if strings.TrimSpace(parseStyleValue(n, "padding-top")) != "" {
		return true
	}
	if strings.TrimSpace(parseStyleValue(n, "padding-bottom")) != "" {
		return true
	}
	return false
}

func parseRowStyle(n *html.Node) Row {
	var r Row

	if h, ok := parseHeightSpec(n); ok && h > 0 {
		r.Height = h
	}

	align := strings.ToLower(strings.TrimSpace(getAttr(n, "align")))
	if align == "" {
		align = strings.ToLower(strings.TrimSpace(parseStyleValue(n, "text-align")))
	}
	switch align {
	case "left", "center", "right":
		r.Align = align
	}

	if v, ok := parseRowVAlignSpec(n); ok {
		r.VAlignSet = true
		r.VAlign = v
	}

	if v := strings.TrimSpace(getAttr(n, "font")); v != "" {
		r.FontFamily = v
	} else if v := strings.TrimSpace(parseStyleValue(n, "font-family")); v != "" {
		r.FontFamily = v
	}

	if v := strings.TrimSpace(getAttr(n, "font-size")); v != "" {
		r.FontSize = parseFontSizeToPoints(v)
	} else if v := strings.TrimSpace(parseStyleValue(n, "font-size")); v != "" {
		r.FontSize = parseFontSizeToPoints(v)
	}

	wsAttr := strings.ToLower(strings.TrimSpace(getAttr(n, "white-space")))
	wsStyle := strings.ToLower(strings.TrimSpace(parseStyleValue(n, "white-space")))
	ws := wsAttr
	if ws == "" {
		ws = wsStyle
	}
	if wsAttr != "" || wsStyle != "" {
		r.NoWrapSet = true
		r.NoWrap = strings.TrimSpace(strings.TrimSuffix(ws, "!important")) == "nowrap"
	}

	weightAttr := strings.TrimSpace(getAttr(n, "font-weight"))
	weightStyle := strings.TrimSpace(parseStyleValue(n, "font-weight"))
	weight := weightAttr
	if weight == "" {
		weight = weightStyle
	}
	if weightAttr != "" || weightStyle != "" {
		r.FontWeightSet = true
		r.FontBold = isBoldFontWeight(weight)
		if r.FontBold {
			r.FontStyle = "B"
		}
	}

	if h, ok := parseLineHeightSpec(n, r.FontSize); ok {
		r.LineHeight = h
	}

	if p := strings.TrimSpace(getAttr(n, "padding")); p != "" {
		pt, pr, pb, pl, err := parsePaddingShorthandToMM(p)
		if err == nil {
			r.Padding = [4]float64{pt, pr, pb, pl}
		}
	} else if p := strings.TrimSpace(parseStyleValue(n, "padding")); p != "" {
		pt, pr, pb, pl, err := parsePaddingShorthandToMM(p)
		if err == nil {
			r.Padding = [4]float64{pt, pr, pb, pl}
		}
	} else {
		pt := parseCSSLengthToMM(parseStyleValue(n, "padding-top"))
		pr := parseCSSLengthToMM(parseStyleValue(n, "padding-right"))
		pb := parseCSSLengthToMM(parseStyleValue(n, "padding-bottom"))
		pl := parseCSSLengthToMM(parseStyleValue(n, "padding-left"))
		r.Padding = [4]float64{pt, pr, pb, pl}
	}

	if ok, bw, bc := parseBorderSpec(n); ok {
		r.BorderSet = true
		r.BorderWidth = bw
		r.BorderColor = bc
	}
	if ok, bw, bc := parseBorderSideSpec(n, "top"); ok {
		r.BorderTopSet = true
		r.BorderTopWidth = bw
		r.BorderTopColor = bc
	}
	if ok, bw, bc := parseBorderSideSpec(n, "right"); ok {
		r.BorderRightSet = true
		r.BorderRightWidth = bw
		r.BorderRightColor = bc
	}
	if ok, bw, bc := parseBorderSideSpec(n, "bottom"); ok {
		r.BorderBottomSet = true
		r.BorderBottomWidth = bw
		r.BorderBottomColor = bc
	}
	if ok, bw, bc := parseBorderSideSpec(n, "left"); ok {
		r.BorderLeftSet = true
		r.BorderLeftWidth = bw
		r.BorderLeftColor = bc
	}

	return r
}

func parseRowVAlignSpec(n *html.Node) (VAlign, bool) {
	if n == nil {
		return VAlignTop, false
	}
	if v := strings.TrimSpace(getAttr(n, "valign")); v != "" {
		return parseRowVAlignValue(v), true
	}
	if v := strings.TrimSpace(parseStyleValue(n, "vertical-align")); v != "" {
		return parseRowVAlignValue(v), true
	}
	return VAlignTop, false
}

func parseRowVAlignValue(v string) VAlign {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
	switch v {
	case "top":
		return VAlignTop
	case "bottom":
		return VAlignBottom
	case "center", "middle":
		return VAlignMiddle
	default:
		return VAlignTop
	}
}
