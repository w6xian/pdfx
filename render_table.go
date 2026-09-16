package pdfx

import (
	"math"
	"sort"
	"strings"

	"github.com/signintech/gopdf"
)

type Table struct {
	X              float64
	Y              float64
	Width          float64
	ColWidths      []float64
	MinRowHeight   float64
	PaddingX       float64
	PaddingY       float64
	LineHeightCoef float64

	BorderWidth float64
	BorderColor [3]uint8
	// BorderAlpha 为 0..1，1 为不透明。小于 1 时描边前临时设置 gopdf 透明度。
	BorderAlpha float64

	FontFamily string
	FontStyle  string
	FontSize   float64

	Header     []string
	Rows       [][]string
	HTML       string
	HeaderRows int

	model []TableRow
}

func (t *Table) Draw(pdf *gopdf.GoPdf) error {
	t.applyDefaultsBasic()
	if err := t.buildModel(); err != nil {
		return err
	}
	t.applyDefaultsPadding()
	t.applyDefaultsBorder()

	placed, rowCount, colCount, err := placeCells(t.model)
	if err != nil {
		return err
	}
	if colCount == 0 || rowCount == 0 {
		return nil
	}
	var fs fontState
	if err := t.ensureColWidthsFromHTML(pdf, placed, colCount, &fs); err != nil {
		return err
	}
	rowMinHeights := make([]float64, rowCount)
	for i := 0; i < rowCount && i < len(t.model); i++ {
		if t.model[i].MinHeightSet {
			rowMinHeights[i] = t.model[i].MinHeight
		}
	}
	rowHeights, err := t.computeRowHeights(pdf, placed, rowCount, rowMinHeights, &fs)
	if err != nil {
		return err
	}
	colX := make([]float64, colCount+1)
	colX[0] = t.X
	for i := 0; i < colCount; i++ {
		colX[i+1] = colX[i] + t.ColWidths[i]
	}
	rowY := make([]float64, rowCount+1)
	rowY[0] = t.Y
	for i := 0; i < rowCount; i++ {
		rowY[i+1] = rowY[i] + rowHeights[i]
	}

	pdf.SetLineWidth(t.BorderWidth)
	pdf.SetStrokeColor(t.BorderColor[0], t.BorderColor[1], t.BorderColor[2])
	if t.BorderAlpha < 1 {
		if tr, err := gopdf.NewTransparency(t.BorderAlpha, string(gopdf.NormalBlendMode)); err == nil {
			_ = pdf.SetTransparency(tr)
		}
	}

	drawn := make(map[lineKey]struct{}, 256)
	lines := make([][4]float64, 0, 256)

	for _, pc := range placed {
		x1 := colX[pc.Col]
		y1 := rowY[pc.Row]
		x2 := colX[pc.Col+pc.ColSpan]
		y2 := rowY[pc.Row+pc.RowSpan]
		t.collectBorderLines(x1, y1, x2, y2, drawn, &lines)
	}
	half := t.BorderWidth / 2.0
	if half < 0 {
		half = 0
	}
	for _, line := range mergeCollinearLines(lines) {
		x1, y1, x2, y2 := extendLineEnds(line[0], line[1], line[2], line[3], half)
		pdf.Line(x1, y1, x2, y2)
	}
	if t.BorderAlpha < 1 {
		// 复位成不透明：alpha 1 时 gopdf 直接把 curr.transparency 置空，
		// 不会多写一个 ExtGState；但不复位就会漏给后面的文字和图片。
		if tr, err := gopdf.NewTransparency(1, string(gopdf.NormalBlendMode)); err == nil {
			_ = pdf.SetTransparency(tr)
		}
	}
	for _, pc := range placed {
		cellFamily := strings.TrimSpace(pc.FontFamily)
		if cellFamily == "" {
			cellFamily = strings.TrimSpace(t.FontFamily)
		}
		if cellFamily == "" {
			if f, ok := getGoPdfCurrentFontFamily(pdf); ok {
				cellFamily = f
			}
		}

		cellStyle := t.FontStyle
		if pc.FontStyle != "" {
			cellStyle = pc.FontStyle
		}

		cellSize := t.FontSize
		if pc.FontSize > 0 {
			cellSize = pc.FontSize
		}
		if cellSize <= 0 {
			if s, ok := getGoPdfCurrentFontSizePoints(pdf); ok {
				cellSize = s
			}
		}
		if cellSize <= 0 {
			cellSize = 9
		}

		fakeBold := false
		if cellFamily != "" && cellSize > 0 {
			var err error
			fakeBold, err = setFontIfNeededWithFakeBold(pdf, cellFamily, cellStyle, cellSize, &fs)
			if err != nil {
				return err
			}
		}

		x1 := colX[pc.Col]
		y1 := rowY[pc.Row]
		x2 := colX[pc.Col+pc.ColSpan]
		y2 := rowY[pc.Row+pc.RowSpan]
		w := x2 - x1
		h := y2 - y1

		if pc.Text == "" {
			continue
		}

		pad := t.cellPadding(pc)
		innerW := w - pad.Left - pad.Right
		if innerW < 0 {
			innerW = 0
		}
		var tLines []string
		if pc.NoWrap {
			tLines = strings.Split(strings.ReplaceAll(strings.ReplaceAll(pc.Text, "\r\n", "\n"), "\r", "\n"), "\n")
		} else {
			var err error
			tLines, err = wrapText(pdf, pc.Text, innerW)
			if err != nil {
				return err
			}
		}
		if len(tLines) == 0 {
			continue
		}

		lineHeight := gopdf.PointsToUnits(gopdf.UnitMM, cellSize*t.LineHeightCoef)
		if pc.LineHeightSet && pc.LineHeight > 0 {
			lineHeight = pc.LineHeight
		}
		textHeight := float64(len(tLines)) * lineHeight
		startY := y1 + pad.Top
		switch pc.VAlign {
		case VAlignMiddle:
			startY = y1 + (h-textHeight)/2
		case VAlignBottom:
			startY = y1 + h - pad.Bottom - textHeight
		}

		for i, line := range tLines {
			yLine := startY + float64(i)*lineHeight
			xLine := x1 + pad.Left
			if pc.Align != AlignLeft {
				tw, err := pdf.MeasureTextWidth(line)
				if err != nil {
					return err
				}
				switch pc.Align {
				case AlignCenter:
					xLine = x1 + (w-tw)/2
				case AlignRight:
					xLine = x1 + w - pad.Right - tw
				}
			}
			pdf.SetXY(xLine, yLine)
			opt := gopdf.CellOption{Align: gopdf.Left | gopdf.Middle, Border: 0, Float: gopdf.Right}
			if err := cellWithOptionRow(pdf, &gopdf.Rect{W: innerW, H: lineHeight}, line, opt, fakeBold); err != nil {
				return err
			}
		}
	}
	pdf.SetXY(t.X, rowY[rowCount]+1)
	return nil
}

type lineKey struct {
	x1, y1, x2, y2 int
}

func (t *Table) collectBorderLines(x1, y1, x2, y2 float64, drawn map[lineKey]struct{}, out *[][4]float64) {
	t.collectLineOnce(x1, y1, x2, y1, drawn, out)
	t.collectLineOnce(x1, y2, x2, y2, drawn, out)
	t.collectLineOnce(x1, y1, x1, y2, drawn, out)
	t.collectLineOnce(x2, y1, x2, y2, drawn, out)
}

func (t *Table) collectLineOnce(x1, y1, x2, y2 float64, drawn map[lineKey]struct{}, out *[][4]float64) {
	k := normalizeLineKey(mmKey(x1), mmKey(y1), mmKey(x2), mmKey(y2))
	if _, ok := drawn[k]; ok {
		return
	}
	drawn[k] = struct{}{}
	*out = append(*out, [4]float64{x1, y1, x2, y2})
}

// markLineDrawn 登记一条线段，返回是否首次出现（false 表示同一条线已画过，应跳过）。
// 相邻的两列会各自画自己那条共用的竖边，重复描边在抗锯齿下会明显更黑更粗。
func markLineDrawn(drawn map[lineKey]struct{}, x1, y1, x2, y2 float64) bool {
	k := normalizeLineKey(mmKey(x1), mmKey(y1), mmKey(x2), mmKey(y2))
	if _, ok := drawn[k]; ok {
		return false
	}
	drawn[k] = struct{}{}
	return true
}

func mmKey(v float64) int {
	return int(math.Round(v * 1000))
}

func normalizeLineKey(x1, y1, x2, y2 int) lineKey {
	if x1 < x2 || (x1 == x2 && y1 <= y2) {
		return lineKey{x1: x1, y1: y1, x2: x2, y2: y2}
	}
	return lineKey{x1: x2, y1: y2, x2: x1, y2: y1}
}

func (t *Table) applyDefaultsBasic() {
	if t.MinRowHeight <= 0 {
		t.MinRowHeight = 6
	}
	if t.LineHeightCoef <= 0 {
		t.LineHeightCoef = 1.25
	}
}

func (t *Table) applyDefaultsPadding() {
	if t.PaddingX <= 0 && t.PaddingY <= 0 {
		t.PaddingX = 1.0
		t.PaddingY = 1.0
		return
	}
	if t.PaddingX <= 0 {
		t.PaddingX = t.PaddingY
	}
	if t.PaddingY <= 0 {
		t.PaddingY = t.PaddingX
	}
}

func (t *Table) applyDefaultsBorder() {
	if t.BorderWidth <= 0 {
		t.BorderWidth = 0.2
	}
	if t.BorderColor == [3]uint8{} {
		t.BorderColor = [3]uint8{0, 0, 0}
	}
	if t.BorderAlpha <= 0 || t.BorderAlpha > 1 {
		t.BorderAlpha = 1
	}
}

func (t *Table) cellPadding(pc PlacedCell) CellPadding {
	if pc.PaddingSet {
		return pc.Padding
	}
	return CellPadding{Left: t.PaddingX, Top: t.PaddingY, Right: t.PaddingX, Bottom: t.PaddingY}
}

type TableRow struct {
	Cells        []TableCell
	MinHeight    float64
	MinHeightSet bool
}

type TableCell struct {
	Text          string
	ColSpan       int
	RowSpan       int
	Align         Align
	VAlign        VAlign
	IsHeader      bool
	FontFamily    string
	FontSize      float64
	FontStyle     string
	Width         float64
	AutoWidth     bool
	NoWrap        bool
	LineHeight    float64
	LineHeightSet bool
	MinHeight     float64
	MinHeightSet  bool
	Padding       CellPadding
	PaddingSet    bool
}

type CellPadding struct {
	Left, Top, Right, Bottom float64
}

type PlacedCell struct {
	TableCell
	Row int
	Col int
}

func placeCells(rows []TableRow) ([]PlacedCell, int, int, error) {
	type occKey struct{ r, c int }
	occupied := make(map[occKey]bool)
	placed := make([]PlacedCell, 0, 32)
	maxRow := 0
	maxCol := 0

	for r, row := range rows {
		c := 0
		for _, cell := range row.Cells {
			cs := cell.ColSpan
			rs := cell.RowSpan
			if cs <= 0 {
				cs = 1
			}
			if rs <= 0 {
				rs = 1
			}

			for occupied[occKey{r: r, c: c}] {
				c++
			}

			pc := PlacedCell{TableCell: cell, Row: r, Col: c}
			pc.ColSpan = cs
			pc.RowSpan = rs
			placed = append(placed, pc)

			for dr := 0; dr < rs; dr++ {
				for dc := 0; dc < cs; dc++ {
					occupied[occKey{r: r + dr, c: c + dc}] = true
				}
			}

			if r+rs > maxRow {
				maxRow = r + rs
			}
			if c+cs > maxCol {
				maxCol = c + cs
			}
			c += cs
		}
		if r+1 > maxRow {
			maxRow = r + 1
		}
	}
	return placed, maxRow, maxCol, nil
}

type TableHTMLMeta struct {
	Width       float64
	CellPadding float64
	BorderWidth float64
	BorderColor [3]uint8
	BorderAlpha float64
}

func (t *Table) buildModel() error {
	if len(t.model) > 0 {
		return nil
	}
	if strings.TrimSpace(t.HTML) != "" {
		rows, headerRows, meta, err := parseHTMLTable(t.HTML, t.FontSize)
		if err != nil {
			return err
		}
		t.model = rows
		if t.HeaderRows == 0 {
			t.HeaderRows = headerRows
		}
		if t.Width <= 0 && meta.Width > 0 {
			t.Width = meta.Width
		}
		if t.PaddingX == 0 && t.PaddingY == 0 && meta.CellPadding > 0 {
			t.PaddingX = meta.CellPadding
			t.PaddingY = meta.CellPadding
		}
		if t.BorderWidth == 0 && meta.BorderWidth > 0 {
			t.BorderWidth = meta.BorderWidth
		}
		if t.BorderColor == [3]uint8{} && meta.BorderColor != [3]uint8{} {
			t.BorderColor = meta.BorderColor
			t.BorderAlpha = meta.BorderAlpha
		}
		return nil
	}
	if len(t.Header) > 0 {
		r := TableRow{}
		for _, s := range t.Header {
			r.Cells = append(r.Cells, TableCell{Text: s, ColSpan: 1, RowSpan: 1, Align: AlignCenter, VAlign: VAlignMiddle, IsHeader: true})
		}
		t.model = append(t.model, r)
		if t.HeaderRows == 0 {
			t.HeaderRows = 1
		}
	}
	for _, rr := range t.Rows {
		r := TableRow{}
		for _, s := range rr {
			r.Cells = append(r.Cells, TableCell{Text: s, ColSpan: 1, RowSpan: 1, Align: AlignLeft, VAlign: VAlignMiddle})
		}
		t.model = append(t.model, r)
	}
	return nil
}

func (t *Table) ensureColWidthsFromHTML(pdf *gopdf.GoPdf, placed []PlacedCell, colCount int, fs *fontState) error {
	if len(t.ColWidths) == colCount {
		return nil
	}
	if t.Width <= 0 {
		if pageW, ok := getGoPdfPageWidthInUnits(pdf); ok && pageW > 0 {
			l, _, r, _ := pdf.Margins()
			t.Width = pageW - l - r
		}
	}
	if t.Width <= 0 {
		t.Width = 100
	}

	if len(t.ColWidths) != colCount {
		t.ColWidths = make([]float64, colCount)
	}

	// 各列的最小内容宽度（只看 colspan=1 的单元格）
	minW, autoRequested, err := t.measureColMinWidths(pdf, placed, colCount, fs)
	if err != nil {
		return err
	}

	explicit := make([]float64, colCount)
	explicitSet := make([]bool, colCount)
	for _, pc := range placed {
		if pc.ColSpan != 1 {
			continue
		}
		if pc.Width > 0 && pc.Width > explicit[pc.Col] {
			explicit[pc.Col] = pc.Width
			explicitSet[pc.Col] = true
		}
	}
	// 跨列单元格上写的 width，作为"这几列合计不小于这个值"的约束
	applyColSpanWidths(placed, colCount, minW, explicit, explicitSet)

	var fixed float64
	var autoCols []int
	for i := 0; i < colCount; i++ {
		if explicitSet[i] {
			t.ColWidths[i] = explicit[i]
			fixed += explicit[i]
		} else {
			autoCols = append(autoCols, i)
		}
	}

	remaining := t.Width - fixed
	if remaining < 0 {
		remaining = 0
	}
	if len(autoCols) == 0 {
		return nil
	}

	reqCols := make([]int, 0, len(autoCols))
	flexCols := make([]int, 0, len(autoCols))
	for _, i := range autoCols {
		if autoRequested[i] {
			reqCols = append(reqCols, i)
		} else {
			flexCols = append(flexCols, i)
		}
	}

	var reqSum float64
	for _, i := range reqCols {
		reqSum += minW[i]
	}
	if reqSum > remaining && reqSum > 0 {
		scale := remaining / reqSum
		for _, i := range reqCols {
			t.ColWidths[i] = minW[i] * scale
		}
		for _, i := range flexCols {
			t.ColWidths[i] = 0
		}
		return nil
	}
	for _, i := range reqCols {
		t.ColWidths[i] = minW[i]
	}
	remaining -= reqSum
	if remaining < 0 {
		remaining = 0
	}

	var flexMinSum float64
	for _, i := range flexCols {
		flexMinSum += minW[i]
	}
	if len(flexCols) == 0 {
		return nil
	}
	if flexMinSum <= 0 {
		each := 0.0
		if len(flexCols) > 0 {
			each = remaining / float64(len(flexCols))
		}
		for _, i := range flexCols {
			t.ColWidths[i] = each
		}
		return nil
	}

	scale := remaining / flexMinSum
	for _, i := range flexCols {
		t.ColWidths[i] = minW[i] * scale
	}
	return nil
}

// measureColMinWidths 量出每一列的最小内容宽度（含左右内边距），
// 顺带记录哪些列显式要求了 auto 宽度。只看 colspan=1 的单元格，
// 跨列单元格的内容暂不分摊到各列上。
func (t *Table) measureColMinWidths(pdf *gopdf.GoPdf, placed []PlacedCell, colCount int, fs *fontState) ([]float64, []bool, error) {
	minW := make([]float64, colCount)
	autoRequested := make([]bool, colCount)
	for _, pc := range placed {
		if pc.ColSpan != 1 || pc.Col < 0 || pc.Col >= colCount {
			continue
		}
		if pc.AutoWidth {
			autoRequested[pc.Col] = true
		}
		cellFamily := strings.TrimSpace(pc.FontFamily)
		if cellFamily == "" {
			cellFamily = strings.TrimSpace(t.FontFamily)
		}
		if cellFamily == "" {
			if f, ok := getGoPdfCurrentFontFamily(pdf); ok {
				cellFamily = f
			}
		}
		cellStyle := t.FontStyle
		if pc.FontStyle != "" {
			cellStyle = pc.FontStyle
		}
		cellSize := t.FontSize
		if pc.FontSize > 0 {
			cellSize = pc.FontSize
		}
		if cellSize <= 0 {
			if s, ok := getGoPdfCurrentFontSizePoints(pdf); ok {
				cellSize = s
			}
		}
		if cellSize <= 0 {
			cellSize = 9
		}
		if cellFamily != "" && cellSize > 0 {
			if err := setFontIfNeeded(pdf, cellFamily, cellStyle, cellSize, fs); err != nil {
				return nil, nil, err
			}
		}
		w, err := pdf.MeasureTextWidth(pc.Text)
		if err != nil {
			return nil, nil, err
		}
		pad := t.cellPadding(pc)
		w += pad.Left + pad.Right
		if w > minW[pc.Col] {
			minW[pc.Col] = w
		}
	}
	return minW, autoRequested, nil
}

// applyColSpanWidths 处理跨列单元格上写的 width。
//
// CSS 里单元格的 width 只是"最小值/建议值"：auto 布局下列的实宽由内容撑开，
// width 只保证"这几列加起来不小于这个值"。之前这里对 pc.ColSpan != 1 的单元格
// 直接 continue，导致 <th colspan="2" width="28mm"> 这类写法被整段丢弃。
//
// 缺口按各列最小内容宽度的比例分摊，而不是平均摊 —— 平均摊会把内容多的列挤爆
// （例如把"滋味、气味"压到 14mm）。跨度内内容已经够宽时不做任何调整。
func applyColSpanWidths(placed []PlacedCell, colCount int, minW []float64, explicit []float64, explicitSet []bool) {
	type spanReq struct {
		cols  []int
		width float64
	}
	reqs := make([]spanReq, 0, 4)
	for _, pc := range placed {
		if pc.ColSpan <= 1 || pc.Width <= 0 || pc.Col < 0 || pc.Col >= colCount {
			continue
		}
		cols := make([]int, 0, pc.ColSpan)
		for i := pc.Col; i < pc.Col+pc.ColSpan && i < colCount; i++ {
			cols = append(cols, i)
		}
		if len(cols) > 1 {
			reqs = append(reqs, spanReq{cols: cols, width: pc.Width})
		}
	}
	// 跨度小的先算，避免大跨度把小跨度的结果盖掉
	sort.SliceStable(reqs, func(i, j int) bool { return len(reqs[i].cols) < len(reqs[j].cols) })

	for _, r := range reqs {
		var cur, basis float64
		free := make([]int, 0, len(r.cols))
		for _, i := range r.cols {
			if explicitSet[i] {
				cur += explicit[i]
				continue
			}
			free = append(free, i)
			basis += minW[i]
		}
		need := r.width - cur
		if need <= 0 || len(free) == 0 {
			continue
		}
		if basis >= need {
			continue // 内容已经撑够了，width 作为最小值已满足
		}
		if basis <= 0 {
			each := need / float64(len(free))
			for _, i := range free {
				if each > minW[i] {
					minW[i] = each
				}
			}
			continue
		}
		scale := need / basis
		for _, i := range free {
			minW[i] *= scale
		}
	}
}

func (t *Table) computeRowHeights(pdf *gopdf.GoPdf, placed []PlacedCell, rowCount int, rowMinHeights []float64, fs *fontState) ([]float64, error) {
	heights := make([]float64, rowCount)
	for i := 0; i < rowCount; i++ {
		minH := t.MinRowHeight
		if i < len(rowMinHeights) && rowMinHeights[i] > 0 {
			minH = rowMinHeights[i]
		}
		heights[i] = minH
	}

	for _, pc := range placed {
		if pc.RowSpan != 1 {
			continue
		}
		cellFamily := strings.TrimSpace(pc.FontFamily)
		if cellFamily == "" {
			cellFamily = strings.TrimSpace(t.FontFamily)
		}
		if cellFamily == "" {
			if f, ok := getGoPdfCurrentFontFamily(pdf); ok {
				cellFamily = f
			}
		}
		cellStyle := t.FontStyle
		if pc.FontStyle != "" {
			cellStyle = pc.FontStyle
		}
		cellSize := t.FontSize
		if pc.FontSize > 0 {
			cellSize = pc.FontSize
		}
		if cellSize <= 0 {
			if s, ok := getGoPdfCurrentFontSizePoints(pdf); ok {
				cellSize = s
			}
		}
		if cellSize <= 0 {
			cellSize = 9
		}
		if cellFamily != "" && cellSize > 0 {
			if err := setFontIfNeeded(pdf, cellFamily, cellStyle, cellSize, fs); err != nil {
				return nil, err
			}
		}

		pad := t.cellPadding(pc)
		w := 0.0
		for i := 0; i < pc.ColSpan; i++ {
			if pc.Col+i < len(t.ColWidths) {
				w += t.ColWidths[pc.Col+i]
			}
		}
		innerW := w - pad.Left - pad.Right
		if innerW < 0 {
			innerW = 0
		}

		var lines []string
		if pc.NoWrap {
			lines = strings.Split(strings.ReplaceAll(strings.ReplaceAll(pc.Text, "\r\n", "\n"), "\r", "\n"), "\n")
		} else {
			var err error
			lines, err = wrapText(pdf, pc.Text, innerW)
			if err != nil {
				return nil, err
			}
		}
		if len(lines) == 0 {
			lines = []string{""}
		}

		lineHeight := gopdf.PointsToUnits(gopdf.UnitMM, cellSize*t.LineHeightCoef)
		if pc.LineHeightSet && pc.LineHeight > 0 {
			lineHeight = pc.LineHeight
		}
		h := float64(len(lines))*lineHeight + pad.Top + pad.Bottom
		if pc.MinHeightSet && pc.MinHeight > h {
			h = pc.MinHeight
		}
		if h > heights[pc.Row] {
			heights[pc.Row] = h
		}
	}

	for _, pc := range placed {
		if pc.RowSpan <= 1 {
			continue
		}
		var sum float64
		for i := 0; i < pc.RowSpan; i++ {
			if pc.Row+i < len(heights) {
				sum += heights[pc.Row+i]
			}
		}
		cellFamily := strings.TrimSpace(pc.FontFamily)
		if cellFamily == "" {
			cellFamily = strings.TrimSpace(t.FontFamily)
		}
		if cellFamily == "" {
			if f, ok := getGoPdfCurrentFontFamily(pdf); ok {
				cellFamily = f
			}
		}
		cellStyle := t.FontStyle
		if pc.FontStyle != "" {
			cellStyle = pc.FontStyle
		}
		cellSize := t.FontSize
		if pc.FontSize > 0 {
			cellSize = pc.FontSize
		}
		if cellSize <= 0 {
			if s, ok := getGoPdfCurrentFontSizePoints(pdf); ok {
				cellSize = s
			}
		}
		if cellSize <= 0 {
			cellSize = 9
		}
		if cellFamily != "" && cellSize > 0 {
			if err := setFontIfNeeded(pdf, cellFamily, cellStyle, cellSize, fs); err != nil {
				return nil, err
			}
		}

		pad := t.cellPadding(pc)
		w := 0.0
		for i := 0; i < pc.ColSpan; i++ {
			if pc.Col+i < len(t.ColWidths) {
				w += t.ColWidths[pc.Col+i]
			}
		}
		innerW := w - pad.Left - pad.Right
		if innerW < 0 {
			innerW = 0
		}

		var lines []string
		if pc.NoWrap {
			lines = strings.Split(strings.ReplaceAll(strings.ReplaceAll(pc.Text, "\r\n", "\n"), "\r", "\n"), "\n")
		} else {
			var err error
			lines, err = wrapText(pdf, pc.Text, innerW)
			if err != nil {
				return nil, err
			}
		}
		if len(lines) == 0 {
			lines = []string{""}
		}
		lineHeight := gopdf.PointsToUnits(gopdf.UnitMM, cellSize*t.LineHeightCoef)
		if pc.LineHeightSet && pc.LineHeight > 0 {
			lineHeight = pc.LineHeight
		}
		h := float64(len(lines))*lineHeight + pad.Top + pad.Bottom
		if pc.MinHeightSet && pc.MinHeight > h {
			h = pc.MinHeight
		}
		if h > sum && pc.RowSpan > 0 {
			extra := (h - sum) / float64(pc.RowSpan)
			for i := 0; i < pc.RowSpan; i++ {
				if pc.Row+i < len(heights) {
					heights[pc.Row+i] += extra
				}
			}
		}
	}

	return heights, nil
}
