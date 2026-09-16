package pdfx

import (
	"strconv"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

func parseBoxWidthToMM(v string, availW float64) float64 {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
	if v == "" {
		return 0
	}
	if strings.HasSuffix(v, "%") {
		if availW <= 0 {
			return 0
		}
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "%")), 64)
		if err != nil || num <= 0 {
			return 0
		}
		return availW * (num / 100.0)
	}
	return parseCSSLengthToMM(v)
}

// colorRGBA 是一次颜色解析的结果，a 取 0..1，1 为完全不透明。
type colorRGBA struct {
	r, g, b uint8
	a       float64
}

func parseColorSpec(v string) ([3]uint8, bool) {
	c, ok := parseColorRGBA(v)
	if !ok {
		return [3]uint8{}, false
	}
	return [3]uint8{c.r, c.g, c.b}, true
}

// parseColorSpecAlpha 与 parseColorSpec 相同，但保留 alpha。
// 带透明度的写法有：8 位十六进制 #RRGGBBAA、4 位 #RGBA、rgba()。
// 只关心 RGB 的调用方继续用 parseColorSpec，alpha 被丢弃。
func parseColorSpecAlpha(v string) ([3]uint8, float64, bool) {
	c, ok := parseColorRGBA(v)
	if !ok {
		return [3]uint8{}, 0, false
	}
	return [3]uint8{c.r, c.g, c.b}, c.a, true
}

func parseColorRGBA(v string) (colorRGBA, bool) {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
	if v == "" {
		return colorRGBA{}, false
	}
	switch v {
	case "black":
		return colorRGBA{a: 1}, true
	case "white":
		return colorRGBA{r: 255, g: 255, b: 255, a: 1}, true
	}
	if strings.HasPrefix(v, "#") {
		h := strings.TrimPrefix(v, "#")
		if len(h) == 3 || len(h) == 4 {
			// #RGB / #RGBA 简写：每位重复一次展开成 #RRGGBB / #RRGGBBAA
			var b strings.Builder
			for i := 0; i < len(h); i++ {
				b.WriteString(strings.Repeat(string(h[i]), 2))
			}
			h = b.String()
		}
		if len(h) != 6 && len(h) != 8 {
			return colorRGBA{}, false
		}
		var rgb [3]uint8
		for i := 0; i < 3; i++ {
			n, err := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
			if err != nil {
				return colorRGBA{}, false
			}
			rgb[i] = uint8(n)
		}
		c := colorRGBA{r: rgb[0], g: rgb[1], b: rgb[2], a: 1}
		if len(h) == 8 {
			n, err := strconv.ParseUint(h[6:8], 16, 8)
			if err != nil {
				return colorRGBA{}, false
			}
			c.a = float64(n) / 255.0
		}
		return c, true
	}
	if (strings.HasPrefix(v, "rgb(") || strings.HasPrefix(v, "rgba(")) && strings.HasSuffix(v, ")") {
		inner := strings.TrimSuffix(v[strings.IndexByte(v, '(')+1:], ")")
		parts := strings.Split(inner, ",")
		if len(parts) != 3 && len(parts) != 4 {
			return colorRGBA{}, false
		}
		var rgb [3]uint8
		for i := 0; i < 3; i++ {
			n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
			if err != nil || n < 0 || n > 255 {
				return colorRGBA{}, false
			}
			rgb[i] = uint8(n)
		}
		c := colorRGBA{r: rgb[0], g: rgb[1], b: rgb[2], a: 1}
		if len(parts) == 4 {
			f, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
			if err != nil || f < 0 || f > 1 {
				return colorRGBA{}, false
			}
			c.a = f
		}
		return c, true
	}
	return colorRGBA{}, false
}

func isBoldFontWeight(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
	switch v {
	case "bold", "bolder":
		return true
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n >= 600
	}
	return false
}

func parseHeightSpec(n *html.Node) (float64, bool) {
	if v := strings.TrimSpace(getAttr(n, "height")); v != "" {
		h := parseCSSLengthToMM(v)
		if h > 0 {
			return h, true
		}
	}
	if v := strings.TrimSpace(parseStyleValue(n, "height")); v != "" {
		h := parseCSSLengthToMM(v)
		if h > 0 {
			return h, true
		}
	}
	return 0, false
}

func parseLineHeightSpec(n *html.Node, fontSizePt float64) (float64, bool) {
	if v := strings.TrimSpace(getAttr(n, "line-height")); v != "" {
		return parseLineHeightValue(v, fontSizePt)
	}
	if v := strings.TrimSpace(parseStyleValue(n, "line-height")); v != "" {
		return parseLineHeightValue(v, fontSizePt)
	}
	return 0, false
}

func parseLineHeightValue(v string, fontSizePt float64) (float64, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == "normal" {
		return 0, false
	}
	if strings.HasSuffix(v, "%") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "%")), 64)
		if err != nil || num <= 0 {
			return 0, false
		}
		mul := num / 100.0
		return gopdf.PointsToUnits(gopdf.UnitMM, fontSizePt*mul), true
	}
	if num, err := strconv.ParseFloat(v, 64); err == nil {
		if num <= 0 {
			return 0, false
		}
		return gopdf.PointsToUnits(gopdf.UnitMM, fontSizePt*num), true
	}
	if h := parseCSSLengthToMM(v); h > 0 {
		return h, true
	}
	return 0, false
}

func parseHexColor(s string) (uint8, uint8, uint8, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, 0, false
	}
	if strings.HasPrefix(s, "#") {
		s = s[1:]
	}
	if len(s) == 3 {
		s = strings.Repeat(string(s[0]), 2) + strings.Repeat(string(s[1]), 2) + strings.Repeat(string(s[2]), 2)
	}
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8((v >> 8) & 0xFF), uint8(v & 0xFF), true
}
