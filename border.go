package pdfx

import (
	"strings"

	"golang.org/x/net/html"
)

func parseBorderSpec(n *html.Node) (ok bool, borderWidth float64, borderColor [3]uint8) {
	attr := strings.TrimSpace(getAttr(n, "border"))
	styleBorder := strings.TrimSpace(parseStyleValue(n, "border"))
	bwAttr := strings.TrimSpace(getAttr(n, "border-width"))
	bwStyle := strings.TrimSpace(parseStyleValue(n, "border-width"))
	bcAttr := strings.TrimSpace(getAttr(n, "border-color"))
	bcStyle := strings.TrimSpace(parseStyleValue(n, "border-color"))

	src := ""
	if attr != "" {
		src = attr
	} else if styleBorder != "" {
		src = styleBorder
	}
	if src != "" {
		return parseBorderValue(src)
	}

	if bwAttr != "" {
		ok = true
		borderWidth = parseCSSLengthToMM(bwAttr)
	} else if bwStyle != "" {
		ok = true
		borderWidth = parseCSSLengthToMM(bwStyle)
	}
	if borderWidth <= 0 && ok {
		borderWidth = 0.2
	}

	colorSrc := ""
	if bcAttr != "" {
		colorSrc = bcAttr
	} else if bcStyle != "" {
		colorSrc = bcStyle
	}
	if colorSrc != "" {
		ok = true
		if c, cOK := parseColorSpec(colorSrc); cOK {
			borderColor = c
		}
	}
	if ok && borderColor == [3]uint8{} {
		borderColor = [3]uint8{0, 0, 0}
	}
	return ok, borderWidth, borderColor
}

const (
	borderSideTop = iota
	borderSideRight
	borderSideBottom
	borderSideLeft
)

func borderSideEffective(style Row, side int) (set bool, width float64, color [3]uint8) {
	switch side {
	case borderSideTop:
		if style.BorderTopSet {
			set = true
			width = style.BorderTopWidth
			color = style.BorderTopColor
		}
	case borderSideRight:
		if style.BorderRightSet {
			set = true
			width = style.BorderRightWidth
			color = style.BorderRightColor
		}
	case borderSideBottom:
		if style.BorderBottomSet {
			set = true
			width = style.BorderBottomWidth
			color = style.BorderBottomColor
		}
	case borderSideLeft:
		if style.BorderLeftSet {
			set = true
			width = style.BorderLeftWidth
			color = style.BorderLeftColor
		}
	}

	if !set && style.BorderSet {
		set = true
		width = style.BorderWidth
		color = style.BorderColor
	}

	if set && color == [3]uint8{} {
		color = [3]uint8{0, 0, 0}
	}
	if width < 0 {
		width = 0
	}
	return set, width, color
}

func borderWidthsEffective(style Row) (top float64, right float64, bottom float64, left float64) {
	_, top, _ = borderSideEffective(style, borderSideTop)
	_, right, _ = borderSideEffective(style, borderSideRight)
	_, bottom, _ = borderSideEffective(style, borderSideBottom)
	_, left, _ = borderSideEffective(style, borderSideLeft)
	if top < 0 {
		top = 0
	}
	if right < 0 {
		right = 0
	}
	if bottom < 0 {
		bottom = 0
	}
	if left < 0 {
		left = 0
	}
	return top, right, bottom, left
}

func parseBorderValue(src string) (ok bool, borderWidth float64, borderColor [3]uint8) {
	src = strings.ToLower(strings.TrimSpace(src))
	src = strings.TrimSpace(strings.TrimSuffix(src, "!important"))
	if src == "" {
		return false, 0, [3]uint8{}
	}
	ok = true
	fields := strings.Fields(src)
	none := false
	wSet := false
	for _, f := range fields {
		f = strings.TrimSpace(strings.TrimSuffix(f, "!important"))
		if f == "" {
			continue
		}
		if f == "none" {
			none = true
			wSet = true
			borderWidth = 0
			continue
		}
		if !wSet {
			wSet = true
			borderWidth = parseCSSLengthToMM(f)
			continue
		}
		if borderColor == [3]uint8{} {
			if c, cOK := parseColorSpec(f); cOK {
				borderColor = c
				continue
			}
		}
	}
	if !wSet {
		borderWidth = 0.2
	} else if borderWidth <= 0 && !none {
		borderWidth = 0.2
	}
	if borderColor == [3]uint8{} && borderWidth > 0 {
		borderColor = [3]uint8{0, 0, 0}
	}
	return ok, borderWidth, borderColor
}

func parseBorderSideSpec(n *html.Node, side string) (ok bool, borderWidth float64, borderColor [3]uint8) {
	if n == nil {
		return false, 0, [3]uint8{}
	}
	side = strings.ToLower(strings.TrimSpace(side))
	if side == "" {
		return false, 0, [3]uint8{}
	}

	attr := strings.TrimSpace(getAttr(n, "border-"+side))
	styleBorder := strings.TrimSpace(parseStyleValue(n, "border-"+side))
	bwAttr := strings.TrimSpace(getAttr(n, "border-"+side+"-width"))
	bwStyle := strings.TrimSpace(parseStyleValue(n, "border-"+side+"-width"))
	bcAttr := strings.TrimSpace(getAttr(n, "border-"+side+"-color"))
	bcStyle := strings.TrimSpace(parseStyleValue(n, "border-"+side+"-color"))

	src := ""
	if attr != "" {
		src = attr
	} else if styleBorder != "" {
		src = styleBorder
	}
	if src != "" {
		return parseBorderValue(src)
	}

	if bwAttr != "" {
		ok = true
		borderWidth = parseCSSLengthToMM(bwAttr)
	} else if bwStyle != "" {
		ok = true
		borderWidth = parseCSSLengthToMM(bwStyle)
	}
	if borderWidth < 0 {
		borderWidth = 0
	}
	if borderWidth <= 0 && ok {
		borderWidth = 0.2
	}

	colorSrc := ""
	if bcAttr != "" {
		colorSrc = bcAttr
	} else if bcStyle != "" {
		colorSrc = bcStyle
	}
	if colorSrc != "" {
		ok = true
		if c, cOK := parseColorSpec(colorSrc); cOK {
			borderColor = c
		}
	}
	if ok && borderColor == [3]uint8{} && borderWidth > 0 {
		borderColor = [3]uint8{0, 0, 0}
	}
	return ok, borderWidth, borderColor
}
