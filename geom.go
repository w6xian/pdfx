package pdfx

import (
	"math"
	"sort"
)

type lineSpan struct{ a, b float64 }

// quantize 把坐标量化到 0.01，用于判断两条线段是否共线
func quantize(v float64) float64 { return math.Round(v*100) / 100 }

// mergeSpans 合并相接或重叠的区间
func mergeSpans(sp []lineSpan) []lineSpan {
	if len(sp) < 2 {
		return sp
	}
	sort.Slice(sp, func(i, j int) bool { return sp[i].a < sp[j].a })
	out := sp[:1]
	for _, s := range sp[1:] {
		last := &out[len(out)-1]
		if s.a <= last.b+0.01 {
			if s.b > last.b {
				last.b = s.b
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// extendLineEnds 把水平/垂直线段的两端各延长 half（通常传线宽的一半）。
//
// gopdf 的 Line() 每次都输出一条独立路径 "m x1 y1 l x2 y2 S"，线端是 butt cap
// （PDF 默认 0 j），且横竖两条线各自成路径、没有 line join。因此表格拐角处
// 那个 w×w 的方块会缺一角：横线只盖住端点右侧，竖线只盖住端点下侧，剩下
// 1/4 谁都没画，放大后就是「角上缺一个像素、没封口」。
//
// 两端各延长 w/2 后，延长量正好落在垂直笔画的外沿：既补上缺口，
// 也不会在 T 形接头处露出多余的线头。gopdf 自带的 tableLayout.drawBorder
// 用的也是这个办法（见 gopdf/table.go），但 Line() 没有暴露 line cap，
// 所以只能在这里自己补坐标。
func extendLineEnds(x1, y1, x2, y2 float64, half float64) (float64, float64, float64, float64) {
	if half <= 0 {
		return x1, y1, x2, y2
	}
	const eps = 0.01
	if math.Abs(y2-y1) <= eps { // 水平线
		if x2 >= x1 {
			return x1 - half, y1, x2 + half, y2
		}
		return x1 + half, y1, x2 - half, y2
	}
	if math.Abs(x2-x1) > eps { // 斜线不处理
		return x1, y1, x2, y2
	}
	if y2 >= y1 { // 垂直线
		return x1, y1 - half, x2, y2 + half
	}
	return x1, y1 + half, x2, y2 - half
}

// mergeCollinearLines 合并同一行/列上重合或相接的线段，
// 避免同一条表格边框被重复描边（重复描边在屏幕上会显得更黑更粗、边缘发灰）
func mergeCollinearLines(lines [][4]float64) [][4]float64 {
	const eps = 0.01
	hor := map[float64][]lineSpan{}
	ver := map[float64][]lineSpan{}
	var ksH, ksV []float64
	for _, l := range lines {
		x1, y1, x2, y2 := l[0], l[1], l[2], l[3]
		if math.Abs(y1-y2) <= eps {
			k := quantize(y1)
			if _, ok := hor[k]; !ok {
				ksH = append(ksH, k)
			}
			a, b := x1, x2
			if a > b {
				a, b = b, a
			}
			hor[k] = append(hor[k], lineSpan{a, b})
			continue
		}
		if math.Abs(x1-x2) > eps {
			continue
		}
		k := quantize(x1)
		if _, ok := ver[k]; !ok {
			ksV = append(ksV, k)
		}
		a, b := y1, y2
		if a > b {
			a, b = b, a
		}
		ver[k] = append(ver[k], lineSpan{a, b})
	}
	out := make([][4]float64, 0, len(lines))
	for _, k := range ksH {
		for _, s := range mergeSpans(hor[k]) {
			out = append(out, [4]float64{s.a, k, s.b, k})
		}
	}
	for _, k := range ksV {
		for _, s := range mergeSpans(ver[k]) {
			out = append(out, [4]float64{k, s.a, k, s.b})
		}
	}
	return out
}
