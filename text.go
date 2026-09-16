package pdfx

import (
	"reflect"
	"strings"

	"github.com/signintech/gopdf"
)

func truncateTextToWidth(pdf *gopdf.GoPdf, text string, maxWidth float64) string {
	text = strings.TrimSpace(text)
	if text == "" || maxWidth <= 0 {
		return ""
	}
	w, err := pdf.MeasureTextWidth(text)
	if err == nil && w <= maxWidth {
		return text
	}

	ellipsis := "…"
	ew, err := pdf.MeasureTextWidth(ellipsis)
	if err != nil {
		ellipsis = ""
		ew = 0
	}
	if ew > maxWidth {
		return ""
	}

	runes := []rune(text)
	lo, hi := 0, len(runes)
	best := 0
	for lo <= hi {
		mid := (lo + hi) / 2
		s := string(runes[:mid]) + ellipsis
		w, err := pdf.MeasureTextWidth(s)
		if err != nil {
			break
		}
		if w <= maxWidth {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best <= 0 {
		return ellipsis
	}
	return string(runes[:best]) + ellipsis
}

func cellWithOptionRow(pdf *gopdf.GoPdf, rect *gopdf.Rect, text string, opt gopdf.CellOption, fakeBold bool) error {
	if !fakeBold {
		return pdf.CellWithOption(rect, text, opt)
	}
	x := pdf.GetX()
	y := pdf.GetY()
	if err := pdf.CellWithOption(rect, text, opt); err != nil {
		return err
	}
	pdf.SetXY(x+0.2, y)
	if err := pdf.CellWithOption(rect, text, opt); err != nil {
		return err
	}
	pdf.SetXY(x, y)
	return nil
}

func wrapText(pdf *gopdf.GoPdf, text string, maxWidth float64) ([]string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if maxWidth <= 0 {
		return []string{text}, nil
	}
	var out []string
	paras := strings.Split(text, "\n")
	for _, para := range paras {
		para = strings.TrimSpace(para)
		if para == "" {
			out = append(out, "")
			continue
		}

		runes := []rune(para)
		start := 0
		lastBreak := -1
		eps := 0.05
		for i := 0; i < len(runes); i++ {
			if runes[i] == ' ' || runes[i] == '\t' {
				lastBreak = i
			}
			line := string(runes[start : i+1])
			w, err := pdf.MeasureTextWidth(line)
			if err != nil {
				return nil, err
			}
			if w <= maxWidth+eps {
				continue
			}
			cut := i
			if lastBreak >= start {
				cut = lastBreak
			}
			if cut == start {
				cut = i + 1
			}
			out = append(out, strings.TrimSpace(string(runes[start:cut])))
			start = cut
			for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t') {
				start++
			}
			i = start - 1
			lastBreak = -1
		}
		if start < len(runes) {
			out = append(out, strings.TrimSpace(string(runes[start:])))
		}
	}
	return out, nil
}

func getGoPdfCurrentFontSizePoints(pdf *gopdf.GoPdf) (float64, bool) {
	rv := reflect.ValueOf(pdf)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return 0, false
	}
	rv = rv.Elem()
	curr := rv.FieldByName("curr")
	if !curr.IsValid() {
		return 0, false
	}
	fs := curr.FieldByName("FontSize")
	if !fs.IsValid() {
		return 0, false
	}
	v := fs.Float()
	if v <= 0 {
		return 0, false
	}
	return v, true
}

func getGoPdfCurrentFontFamily(pdf *gopdf.GoPdf) (string, bool) {
	rv := reflect.ValueOf(pdf)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return "", false
	}
	rv = rv.Elem()
	curr := rv.FieldByName("curr")
	if !curr.IsValid() {
		return "", false
	}
	fi := curr.FieldByName("FontISubset")
	if !fi.IsValid() || (fi.Kind() == reflect.Interface && fi.IsNil()) {
		return "", false
	}
	if fi.Kind() == reflect.Interface {
		fi = fi.Elem()
	}
	// curr 和 FontISubset 都是 gopdf 的未导出字段，反射取到的值带 flagRO：
	// CanInterface() 恒为 false，下面的 Interface() 拿不到东西，
	// MethodByName(...).Call() 更是直接 panic（reflect: mustBeExported）。
	// 读不出来就放弃，交给调用方走默认值 —— 设置过字体时必走这里。
	if !fi.CanInterface() {
		return "", false
	}
	if fi.IsValid() && fi.CanInterface() {
		if sub, ok := fi.Interface().(*gopdf.SubsetFontObj); ok {
			f := strings.TrimSpace(sub.GetFamily())
			if f != "" {
				return f, true
			}
		}
	}
	m := fi.MethodByName("GetFamily")
	if m.IsValid() {
		out := m.Call(nil)
		if len(out) == 1 {
			if s, ok := out[0].Interface().(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s, true
				}
			}
		}
	}
	return "", false
}
