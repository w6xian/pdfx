package pdfx

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

func DrawRowFromString(pdf *gopdf.GoPdf, htmlStr string) error {
	htmlStr = normalizeRowCustomTags(htmlStr)
	rowNode, err := findFirstElementFragment(htmlStr, "row")
	if err != nil {
		return err
	}
	if rowNode == nil {
		return errors.New("row: tag not found")
	}
	return DrawRowFromNode(pdf, rowNode)
}

func DrawRowFromNode(pdf *gopdf.GoPdf, rowNode *html.Node) error {
	if rowNode == nil || rowNode.Type != html.ElementNode || !strings.EqualFold(rowNode.Data, "row") {
		return errors.New("row: invalid node")
	}

	rowStartY := pdf.GetY()

	row := parseRowNode(rowNode)
	if len(row.Cols) > 0 {
		return drawRowCols(pdf, rowStartY, row, rowNode)
	}
	text := strings.TrimSpace(extractText(rowNode))

	family := row.FontFamily
	style := row.FontStyle
	size := row.FontSize
	if family == "" {
		family = "songti"
	}
	if size <= 0 {
		size = 9
	}
	fakeBold := false
	if err := pdf.SetFont(family, style, size); err != nil {
		if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(style), "B") {
			if err2 := pdf.SetFont(family, "", size); err2 != nil {
				return err2
			}
			fakeBold = true
		} else {
			return err
		}
	}

	lineHeight := row.LineHeight
	if lineHeight <= 0 {
		if row.FontSize > 0 {
			lineHeight = pdf.PointsToUnits(row.FontSize * 1.25)
		}
		if lineHeight <= 0 {
			lineHeight = pdf.PointsToUnits(9 * 1.25)
		}
	}

	lines := []string{text}
	if strings.Contains(text, "\n") {
		lines = strings.Split(text, "\n")
	}

	textBlockH := lineHeight * float64(len(lines))
	paddingTop := row.Padding[0]
	paddingRight := row.Padding[1]
	paddingBottom := row.Padding[2]
	paddingLeft := row.Padding[3]

	leftM, _, rightM, _ := pdf.Margins()
	pageW, ok := getGoPdfPageWidthInUnits(pdf)
	if !ok || pageW <= 0 {
		pageW = leftM + rightM + 100
	}
	rowW := pageW - leftM - rightM
	contentW := rowW - paddingLeft - paddingRight
	if contentW < 0 {
		contentW = 0
	}

	rowH := row.Height
	minH := paddingTop + paddingBottom + textBlockH
	if rowH <= 0 {
		rowH = minH
	}

	rowStartY = ensurePageSpace(pdf, rowH)

	contentH := rowH - paddingTop - paddingBottom
	if contentH < 0 {
		contentH = 0
	}
	topOffset := 0.0
	if contentH > textBlockH {
		topOffset = (contentH - textBlockH) / 2.0
	}

	x0 := leftM + paddingLeft
	y0 := rowStartY + paddingTop + topOffset

	align := gopdf.Left
	switch strings.ToLower(strings.TrimSpace(row.Align)) {
	case "center":
		align = gopdf.Center
	case "right":
		align = gopdf.Right
	}

	maxLines := len(lines)
	if row.Height > 0 && lineHeight > 0 {
		fit := int(math.Floor((contentH + 1e-9) / lineHeight))
		if fit < 0 {
			fit = 0
		}
		if fit < maxLines {
			maxLines = fit
		}
	}

	for i := 0; i < maxLines; i++ {
		line := lines[i]
		pdf.SetX(x0)
		pdf.SetY(y0 + float64(i)*lineHeight)
		opt := gopdf.CellOption{Align: align | gopdf.Top}
		if err := cellWithOptionRow(pdf, &gopdf.Rect{W: contentW, H: lineHeight}, line, opt, fakeBold); err != nil {
			return err
		}
	}

	pdf.SetXY(leftM, rowStartY+rowH)
	return nil
}

func drawRowCols(pdf *gopdf.GoPdf, rowStartY float64, row Row, rowNode *html.Node) error {
	leftM, _, rightM, _ := pdf.Margins()
	pageW, ok := getGoPdfPageWidthInUnits(pdf)
	if !ok || pageW <= 0 {
		pageW = leftM + rightM + 100
	}
	rowW := pageW - leftM - rightM
	if rowW < 0 {
		rowW = 0
	}

	type colLayout struct {
		x        float64
		w        float64
		style    Row
		text     string
		box      *Box
		lh       float64
		padTop   float64
		padRight float64
		padBot   float64
		padLeft  float64
		align    int
		fakeBold bool
	}

	widths := make([]float64, len(row.Cols))
	percent := make([]float64, len(row.Cols))
	hasPercent := make([]bool, len(row.Cols))
	autoWanted := make([]bool, len(row.Cols))
	fixedSum := 0.0
	unspecified := 0
	autoReq := make([]float64, len(row.Cols))
	autoIdxAuto := make([]int, 0, len(row.Cols))

	defaultNoWrap := true
	if !row.NoWrapSet && len(row.Cols) == 1 && strings.TrimSpace(row.Cols[0].Width) == "" {
		defaultNoWrap = false
	}
	if !row.NoWrapSet {
		row.NoWrap = defaultNoWrap
	}

	for i, c := range row.Cols {
		wStr := strings.TrimSpace(c.Width)
		if wStr == "" {
			unspecified++
			continue
		}
		if isAutoWidthSpec(wStr) {
			unspecified++
			autoWanted[i] = true
			continue
		}
		if strings.HasSuffix(strings.TrimSpace(wStr), "%") {
			num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(wStr), "%")), 64)
			if err == nil && num > 0 {
				hasPercent[i] = true
				percent[i] = num / 100.0
				continue
			}
			unspecified++
			continue
		}
		mm := parseCSSLengthToMM(wStr)
		if mm > 0 {
			widths[i] = mm
			fixedSum += mm
		} else {
			unspecified++
		}
	}

	for i := range widths {
		if hasPercent[i] {
			widths[i] = rowW * percent[i]
			fixedSum += widths[i]
		}
	}

	if fixedSum > rowW && fixedSum > 0 {
		scale := rowW / fixedSum
		for i := range widths {
			if widths[i] > 0 {
				widths[i] *= scale
			}
		}
		fixedSum = rowW
	}

	layouts := make([]colLayout, 0, len(row.Cols))
	x := leftM
	maxNeedH := 0.0

	for i, c := range row.Cols {
		w := widths[i]
		if w < 0 {
			w = 0
		}

		style := row
		if c.Height > 0 {
			style.Height = c.Height
		}
		if c.LineHeight > 0 {
			style.LineHeight = c.LineHeight
		}
		if c.Align != "" {
			style.Align = c.Align
		}
		if c.FontFamily != "" {
			style.FontFamily = c.FontFamily
		}
		if c.FontSize > 0 {
			style.FontSize = c.FontSize
		}
		if c.FontStyle != "" {
			style.FontStyle = c.FontStyle
		}
		if c.FontWeightSet {
			style.FontWeightSet = true
			style.FontBold = c.FontBold
			if c.FontBold {
				style.FontStyle = "B"
			} else {
				style.FontStyle = ""
			}
		}
		if c.NoWrapSet {
			style.NoWrapSet = true
			style.NoWrap = c.NoWrap
		} else if !style.NoWrapSet {
			style.NoWrap = row.NoWrap
		}
		if c.PaddingSet {
			style.Padding = c.Padding
		}
		if c.BorderSet {
			style.BorderSet = true
			style.BorderWidth = c.BorderWidth
			style.BorderColor = c.BorderColor
			style.BorderTopSet = false
			style.BorderRightSet = false
			style.BorderBottomSet = false
			style.BorderLeftSet = false
		}
		if c.BorderTopSet {
			style.BorderTopSet = true
			style.BorderTopWidth = c.BorderTopWidth
			style.BorderTopColor = c.BorderTopColor
		}
		if c.BorderRightSet {
			style.BorderRightSet = true
			style.BorderRightWidth = c.BorderRightWidth
			style.BorderRightColor = c.BorderRightColor
		}
		if c.BorderBottomSet {
			style.BorderBottomSet = true
			style.BorderBottomWidth = c.BorderBottomWidth
			style.BorderBottomColor = c.BorderBottomColor
		}
		if c.BorderLeftSet {
			style.BorderLeftSet = true
			style.BorderLeftWidth = c.BorderLeftWidth
			style.BorderLeftColor = c.BorderLeftColor
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

		fakeBold := false
		if err := pdf.SetFont(family, fontStyle, fontSize); err != nil {
			if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fontStyle), "B") {
				if err2 := pdf.SetFont(family, "", fontSize); err2 != nil {
					return err2
				}
				fakeBold = true
			} else {
				return err
			}
		}

		lh := style.LineHeight
		if lh <= 0 {
			lh = pdf.PointsToUnits(fontSize * 1.25)
		}

		padTop := style.Padding[0]
		padRight := style.Padding[1]
		padBot := style.Padding[2]
		padLeft := style.Padding[3]

		innerW := w - padLeft - padRight
		if innerW < 0 {
			innerW = 0
		}

		txt := collapseSpace(c.Text)
		if c.Box != nil {
			txt = collapseSpace(c.Box.Text)
		}
		needH := 0.0
		if c.Box != nil {
			_, _, _, _, boxH, _, err := computeBoxLayout(pdf, style, c.Box, innerW)
			if err != nil {
				return err
			}
			needH = padTop + padBot + boxH
			if autoWanted[i] {
				_, _, _, boxW, _, _, err := computeBoxLayout(pdf, style, c.Box, 0)
				if err != nil {
					return err
				}
				autoReq[i] = boxW + padLeft + padRight
				autoIdxAuto = append(autoIdxAuto, i)
			}
		} else {
			if style.NoWrap {
				needH = padTop + padBot + lh
			} else {
				lines, err := wrapText(pdf, txt, innerW)
				if err != nil {
					return err
				}
				if len(lines) == 0 {
					lines = []string{""}
				}
				needH = padTop + padBot + float64(len(lines))*lh
			}
			if autoWanted[i] {
				reqW, err := pdf.MeasureTextWidth(txt)
				if err != nil {
					return err
				}
				autoReq[i] = reqW + padLeft + padRight
				autoIdxAuto = append(autoIdxAuto, i)
			}
		}
		if needH > maxNeedH {
			maxNeedH = needH
		}

		align := gopdf.Left
		switch strings.ToLower(strings.TrimSpace(style.Align)) {
		case "center":
			align = gopdf.Center
		case "right":
			align = gopdf.Right
		}

		layouts = append(layouts, colLayout{
			x:        x,
			w:        w,
			style:    style,
			text:     txt,
			box:      c.Box,
			lh:       lh,
			padTop:   padTop,
			padRight: padRight,
			padBot:   padBot,
			padLeft:  padLeft,
			align:    align,
			fakeBold: fakeBold,
		})

		x += w
	}

	if unspecified > 0 {
		remain := rowW - fixedSum
		if remain < 0 {
			remain = 0
		}

		allocateAuto := func(idxs []int) {
			for _, idx := range idxs {
				if remain <= 0 {
					widths[idx] = 0
					continue
				}
				w := autoReq[idx]
				if w <= 0 {
					w = 0
				}
				if w > remain {
					w = remain
				}
				widths[idx] = w
				remain -= w
			}
		}

		if row.NoWrap {
			allocateAuto(autoIdxAuto)
			flex := 0
			for i := range widths {
				if hasPercent[i] || widths[i] > 0 || autoWanted[i] {
					continue
				}
				flex++
			}
			autoW := 0.0
			if flex > 0 {
				autoW = remain / float64(flex)
			}
			for i := range widths {
				if widths[i] <= 0 && !hasPercent[i] && !autoWanted[i] {
					widths[i] = autoW
				}
			}
		} else {
			allocateAuto(autoIdxAuto)
			flex := 0
			for i := range widths {
				if hasPercent[i] || widths[i] > 0 || autoWanted[i] {
					continue
				}
				flex++
			}
			autoW := 0.0
			if flex > 0 {
				autoW = remain / float64(flex)
			}
			for i := range widths {
				if widths[i] <= 0 && !hasPercent[i] && !autoWanted[i] {
					widths[i] = autoW
				}
			}
		}

		x = leftM
		for i := range layouts {
			if i < len(widths) {
				layouts[i].x = x
				layouts[i].w = widths[i]
				x += widths[i]
			}
		}
	}

	rowH := row.Height
	if rowH <= 0 {
		maxNeedH = 0
		for _, lay := range layouts {
			needH := 0.0
			if lay.box != nil {
				innerW := lay.w - lay.padLeft - lay.padRight
				if innerW < 0 {
					innerW = 0
				}
				_, _, _, _, boxH, _, err := computeBoxLayout(pdf, lay.style, lay.box, innerW)
				if err != nil {
					return err
				}
				needH = lay.padTop + lay.padBot + boxH
			} else {
				family := lay.style.FontFamily
				fontStyle := lay.style.FontStyle
				fontSize := lay.style.FontSize
				if family == "" {
					family = "songti"
				}
				if fontSize <= 0 {
					fontSize = 9
				}
				if err := pdf.SetFont(family, fontStyle, fontSize); err != nil {
					if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fontStyle), "B") {
						if err2 := pdf.SetFont(family, "", fontSize); err2 != nil {
							return err2
						}
					} else {
						return err
					}
				}

				if lay.style.NoWrap {
					needH = lay.padTop + lay.padBot + lay.lh
				} else {
					innerW := lay.w - lay.padLeft - lay.padRight
					if innerW < 0 {
						innerW = 0
					}
					lines, err := wrapText(pdf, lay.text, innerW)
					if err != nil {
						return err
					}
					if len(lines) == 0 {
						lines = []string{""}
					}
					needH = lay.padTop + lay.padBot + float64(len(lines))*lay.lh
				}
			}
			if needH > maxNeedH {
				maxNeedH = needH
			}
		}
		rowH = maxNeedH
	}

	rowStartY = ensurePageSpace(pdf, rowH)

	drawnBorder := make(map[lineKey]struct{}, 16)
	strokeBorder := func(x1, y1, x2, y2, bw float64, bc [3]uint8) {
		if bw <= 0 {
			return
		}
		if !markLineDrawn(drawnBorder, x1, y1, x2, y2) {
			return
		}
		pdf.SetLineWidth(bw)
		pdf.SetStrokeColor(bc[0], bc[1], bc[2])
		ex1, ey1, ex2, ey2 := extendLineEnds(x1, y1, x2, y2, bw/2.0)
		pdf.Line(ex1, ey1, ex2, ey2)
	}

	for _, lay := range layouts {
		if lay.w > 0 && rowH > 0 {
			x1 := lay.x
			y1 := rowStartY
			x2 := lay.x + lay.w
			y2 := rowStartY + rowH

			if _, bw, bc := borderSideEffective(lay.style, borderSideTop); bw > 0 {
				strokeBorder(x1, y1, x2, y1, bw, bc)
			}
			if _, bw, bc := borderSideEffective(lay.style, borderSideBottom); bw > 0 {
				strokeBorder(x1, y2, x2, y2, bw, bc)
			}
			if _, bw, bc := borderSideEffective(lay.style, borderSideLeft); bw > 0 {
				strokeBorder(x1, y1, x1, y2, bw, bc)
			}
			if _, bw, bc := borderSideEffective(lay.style, borderSideRight); bw > 0 {
				strokeBorder(x2, y1, x2, y2, bw, bc)
			}
		}

		colContentH := rowH - lay.padTop - lay.padBot
		if colContentH < 0 {
			colContentH = 0
		}
		colY := rowStartY + lay.padTop
		colVA := VAlignTop
		if lay.style.VAlignSet {
			colVA = lay.style.VAlign
		}

		if lay.box != nil {
			y0 := rowStartY + lay.padTop
			innerW := lay.w - lay.padLeft - lay.padRight
			if innerW < 0 {
				innerW = 0
			}

			boxStyle, boxLines, boxLh, boxW, boxH, boxFakeBold, err := computeBoxLayout(pdf, lay.style, lay.box, innerW)
			if err != nil {
				return err
			}
			bwTop, bwRight, bwBottom, bwLeft := borderWidthsEffective(boxStyle)

			xBox := lay.x + lay.padLeft
			if innerW > boxW {
				switch lay.align {
				case gopdf.Center:
					xBox += (innerW - boxW) / 2
				case gopdf.Right:
					xBox += innerW - boxW
				}
			}
			yBox := y0
			if colVA == VAlignMiddle {
				if colContentH > boxH {
					yBox += (colContentH - boxH) / 2
				}
			} else if colVA == VAlignBottom {
				if colContentH > boxH {
					yBox += colContentH - boxH
				}
			}

			if boxW > 0 && boxH > 0 {
				x1 := xBox
				y1 := yBox
				x2 := xBox + boxW
				y2 := yBox + boxH
				if bwTop > 0 {
					_, bw, bc := borderSideEffective(boxStyle, borderSideTop)
					strokeBorder(x1, y1, x2, y1, bw, bc)
				}
				if bwBottom > 0 {
					_, bw, bc := borderSideEffective(boxStyle, borderSideBottom)
					strokeBorder(x1, y2, x2, y2, bw, bc)
				}
				if bwLeft > 0 {
					_, bw, bc := borderSideEffective(boxStyle, borderSideLeft)
					strokeBorder(x1, y1, x1, y2, bw, bc)
				}
				if bwRight > 0 {
					_, bw, bc := borderSideEffective(boxStyle, borderSideRight)
					strokeBorder(x2, y1, x2, y2, bw, bc)
				}
			}

			family := boxStyle.FontFamily
			fontStyle := boxStyle.FontStyle
			fontSize := boxStyle.FontSize
			if family == "" {
				family = "songti"
			}
			if fontSize <= 0 {
				fontSize = 9
			}
			fakeBold := boxFakeBold
			if err := pdf.SetFont(family, fontStyle, fontSize); err != nil {
				if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fontStyle), "B") {
					if err2 := pdf.SetFont(family, "", fontSize); err2 != nil {
						return err2
					}
					fakeBold = true
					fontStyle = ""
				} else {
					return err
				}
			}

			padTop := boxStyle.Padding[0]
			padRight := boxStyle.Padding[1]
			padBot := boxStyle.Padding[2]
			padLeft := boxStyle.Padding[3]
			contentX := xBox + bwLeft + padLeft
			contentW := boxW - bwLeft - bwRight - padLeft - padRight
			if contentW < 0 {
				contentW = 0
			}
			contentH := boxH - bwTop - bwBottom - padTop - padBot
			if contentH < 0 {
				contentH = 0
			}
			contentY := yBox + bwTop + padTop

			textAlign := gopdf.Left
			switch strings.ToLower(strings.TrimSpace(boxStyle.Align)) {
			case "center":
				textAlign = gopdf.Center
			case "right":
				textAlign = gopdf.Right
			}

			if boxStyle.NoWrap {
				fittedFontSize := fontSize
				fittedStyle := fontStyle
				fittedFakeBold := fakeBold
				txt := ""
				if len(boxLines) > 0 {
					txt = boxLines[0]
				}
				if contentW > 0 && strings.TrimSpace(txt) != "" {
					if err := pdf.SetFont(family, fontStyle, fontSize); err != nil {
						if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fontStyle), "B") {
							if err2 := pdf.SetFont(family, "", fontSize); err2 != nil {
								return err2
							}
							fittedFakeBold = true
							fittedStyle = ""
						} else {
							return err
						}
					}
					w, err := pdf.MeasureTextWidth(txt)
					if err != nil {
						return err
					}
					if w > contentW {
						ratio := contentW / w
						if ratio < 1 {
							newSize := fontSize * ratio
							if newSize < 6 {
								newSize = 6
							}
							fittedFontSize = newSize
							if err := pdf.SetFont(family, fittedStyle, fittedFontSize); err != nil {
								if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fittedStyle), "B") {
									if err2 := pdf.SetFont(family, "", fittedFontSize); err2 != nil {
										return err2
									}
									fittedFakeBold = true
									fittedStyle = ""
								} else {
									return err
								}
							}
							w2, err := pdf.MeasureTextWidth(txt)
							if err != nil {
								return err
							}
							if w2 > contentW {
								txt = truncateTextToWidth(pdf, txt, contentW)
							}
						}
					}
				} else {
					if err := pdf.SetFont(family, fittedStyle, fittedFontSize); err != nil {
						if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fittedStyle), "B") {
							if err2 := pdf.SetFont(family, "", fittedFontSize); err2 != nil {
								return err2
							}
							fittedFakeBold = true
							fittedStyle = ""
						} else {
							return err
						}
					}
				}

				va := VAlignTop
				if boxStyle.VAlignSet {
					va = boxStyle.VAlign
				}
				if va == VAlignMiddle {
					if contentH > boxLh {
						contentY += (contentH - boxLh) / 2
					}
				} else if va == VAlignBottom {
					if contentH > boxLh {
						contentY += contentH - boxLh
					}
				}

				pdf.SetXY(contentX, contentY)
				opt := gopdf.CellOption{Align: textAlign | gopdf.Top}
				if err := cellWithOptionRow(pdf, &gopdf.Rect{W: contentW, H: boxLh}, txt, opt, fittedFakeBold); err != nil {
					return err
				}
			} else {
				maxLines := len(boxLines)
				if boxStyle.Height > 0 && boxLh > 0 {
					fit := int(math.Floor((contentH + 1e-9) / boxLh))
					if fit < 0 {
						fit = 0
					}
					if fit < maxLines {
						maxLines = fit
					}
				}
				textBlockH := float64(maxLines) * boxLh
				va := VAlignTop
				if boxStyle.VAlignSet {
					va = boxStyle.VAlign
				}
				if va == VAlignMiddle {
					if contentH > textBlockH {
						contentY += (contentH - textBlockH) / 2
					}
				} else if va == VAlignBottom {
					if contentH > textBlockH {
						contentY += contentH - textBlockH
					}
				}
				for i := 0; i < maxLines; i++ {
					pdf.SetXY(contentX, contentY+float64(i)*boxLh)
					opt := gopdf.CellOption{Align: textAlign | gopdf.Top}
					if err := cellWithOptionRow(pdf, &gopdf.Rect{W: contentW, H: boxLh}, boxLines[i], opt, fakeBold); err != nil {
						return err
					}
				}
			}

			continue
		}

		family := lay.style.FontFamily
		fontStyle := lay.style.FontStyle
		fontSize := lay.style.FontSize
		if family == "" {
			family = "songti"
		}
		if fontSize <= 0 {
			fontSize = 9
		}

		y0 := rowStartY + lay.padTop
		innerW := lay.w - lay.padLeft - lay.padRight
		if innerW < 0 {
			innerW = 0
		}

		if lay.style.NoWrap {
			y0 = colY
			if colVA == VAlignMiddle {
				if colContentH > lay.lh {
					y0 += (colContentH - lay.lh) / 2
				}
			} else if colVA == VAlignBottom {
				if colContentH > lay.lh {
					y0 += colContentH - lay.lh
				}
			}

			fittedFontSize := fontSize
			fittedStyle := fontStyle
			fittedFakeBold := lay.fakeBold
			txt := lay.text
			if innerW > 0 && strings.TrimSpace(txt) != "" {
				if err := pdf.SetFont(family, fontStyle, fontSize); err != nil {
					if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fontStyle), "B") {
						if err2 := pdf.SetFont(family, "", fontSize); err2 != nil {
							return err2
						}
						fittedFakeBold = true
						fittedStyle = ""
					} else {
						return err
					}
				}
				w, err := pdf.MeasureTextWidth(txt)
				if err != nil {
					return err
				}
				if w > innerW {
					ratio := innerW / w
					if ratio < 1 {
						newSize := fontSize * ratio
						if newSize < 6 {
							newSize = 6
						}
						fittedFontSize = newSize
						if err := pdf.SetFont(family, fittedStyle, fittedFontSize); err != nil {
							if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fittedStyle), "B") {
								if err2 := pdf.SetFont(family, "", fittedFontSize); err2 != nil {
									return err2
								}
								fittedFakeBold = true
								fittedStyle = ""
							} else {
								return err
							}
						}
						w2, err := pdf.MeasureTextWidth(txt)
						if err != nil {
							return err
						}
						if w2 > innerW {
							txt = truncateTextToWidth(pdf, txt, innerW)
						}
					}
				}
			} else {
				if err := pdf.SetFont(family, fittedStyle, fittedFontSize); err != nil {
					if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fittedStyle), "B") {
						if err2 := pdf.SetFont(family, "", fittedFontSize); err2 != nil {
							return err2
						}
						fittedFakeBold = true
						fittedStyle = ""
					} else {
						return err
					}
				}
			}

			pdf.SetXY(lay.x+lay.padLeft, y0)
			opt := gopdf.CellOption{Align: lay.align | gopdf.Top}
			if err := cellWithOptionRow(pdf, &gopdf.Rect{W: innerW, H: lay.lh}, txt, opt, fittedFakeBold); err != nil {
				return err
			}
		} else {
			if err := pdf.SetFont(family, fontStyle, fontSize); err != nil {
				if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(fontStyle), "B") {
					if err2 := pdf.SetFont(family, "", fontSize); err2 != nil {
						return err2
					}
				} else {
					return err
				}
			}

			lines, err := wrapText(pdf, lay.text, innerW)
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				lines = []string{""}
			}
			maxLines := len(lines)
			contentH := colContentH
			if row.Height > 0 && lay.lh > 0 {
				fit := int(math.Floor((contentH + 1e-9) / lay.lh))
				if fit < 0 {
					fit = 0
				}
				if fit < maxLines {
					maxLines = fit
				}
			}
			textBlockH := float64(maxLines) * lay.lh
			y0 = colY
			if colVA == VAlignMiddle {
				if contentH > textBlockH {
					y0 += (contentH - textBlockH) / 2
				}
			} else if colVA == VAlignBottom {
				if contentH > textBlockH {
					y0 += contentH - textBlockH
				}
			}
			for i := 0; i < maxLines; i++ {
				pdf.SetXY(lay.x+lay.padLeft, y0+float64(i)*lay.lh)
				opt := gopdf.CellOption{Align: lay.align | gopdf.Top}
				if err := cellWithOptionRow(pdf, &gopdf.Rect{W: innerW, H: lay.lh}, lines[i], opt, lay.fakeBold); err != nil {
					return err
				}
			}
		}
	}

	pdf.SetXY(leftM, rowStartY+rowH)
	_ = rowNode
	return nil
}

func ensurePageSpace(pdf *gopdf.GoPdf, needH float64) float64 {
	if pdf == nil || needH <= 0 {
		return pdf.GetY()
	}
	pageH, ok := getGoPdfPageHeightInUnits(pdf)
	if !ok || pageH <= 0 {
		return pdf.GetY()
	}
	topM := pdf.MarginTop()
	bottomM := pdf.MarginBottom()
	y := pdf.GetY()
	if y+needH <= pageH-bottomM+1e-6 {
		return y
	}
	pdf.AddPage()
	pdf.SetXY(pdf.MarginLeft(), topM)
	return topM
}
