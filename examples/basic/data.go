package main

import (
	"strings"
)

// Item 检验项目（JSON 字段与 template/inspection 保持一致）
type Item struct {
	Group     string `json:"group,omitempty"`     // 组名（合并单元格），如 "感观"
	GroupSpan int    `json:"groupSpan,omitempty"` // 组占几行
	Sub       string `json:"sub,omitempty"`       // 组内子项名，如 "色泽"
	Name      string `json:"name,omitempty"`      // 普通项名称（占左侧两列）
	Standard  string `json:"standard"`            // 标准规定
	Result    string `json:"result"`              // 检验结果
	Verdict   string `json:"verdict"`             // 单项结论
}

// Data 出厂检验报告数据
type Data struct {
	Company     string `json:"company"`
	Title       string `json:"title"`
	Number      string `json:"number"`
	ProductName string `json:"productName"`
	Model       string `json:"model"`
	Sampler     string `json:"sampler"`
	SamplePlace string `json:"samplePlace"`
	SampleDate  string `json:"sampleDate"`
	TestDate    string `json:"testDate"`
	ProductDate string `json:"productDate"`
	ShelfDate   string `json:"shelfDate"`
	SampleQty   string `json:"sampleQty"`
	NetWeight   string `json:"netWeight"`
	Basis       string `json:"basis"`
	Items       []Item `json:"items"`
	Conclusion  string `json:"conclusion"`
	Reviewer    string `json:"reviewer,omitempty"`
	Inspector   string `json:"inspector,omitempty"`
	ReviewerSig string `json:"reviewerSig,omitempty"` // 审核签名图（素材名或绝对路径）
	Seal        string `json:"seal,omitempty"`        // 印章图（素材名或绝对路径）
	Checker     string `json:"checker,omitempty"`     // 检验员签名图（素材名或绝对路径）
}

// rowView 模板中一行的视图：合并单元格在 Go 侧折算好，模板只管输出 td
type rowView struct {
	Group    string // 组名，非空时输出 rowspan 单元格
	Span     int
	Sub      string // 子项，非空时输出子项单元格
	Name     string // 普通项名，非空时输出 colspan=2 单元格
	Standard string
	Result   string
	Verdict  string
}

// tplData 传给 HTML 模板的顶层数据
type tplData struct {
	Data
	Rows           []rowView
	ConclusionText string
}

func buildRows(items []Item) []rowView {
	rows := make([]rowView, 0, len(items))
	remain := 0 // 组 rowspan 剩余行数（剩余行不再输出第 1 列）
	for _, it := range items {
		r := rowView{Standard: it.Standard, Result: it.Result, Verdict: it.Verdict}
		switch {
		case strings.TrimSpace(it.Group) != "":
			span := it.GroupSpan
			if span < 1 {
				span = 1
			}
			r.Group = flat(it.Group)
			r.Span = span
			r.Sub = it.Sub
			remain = span - 1
		case remain > 0:
			remain--
			r.Sub = it.Sub
		default:
			r.Name = it.Name
		}
		rows = append(rows, r)
	}
	return rows
}

// flat 去掉换行（表格单元格不支持竖排文字）
func flat(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.TrimSpace(s)
}

func conclusionText(d *Data) string {
	c := strings.TrimSpace(d.Conclusion)
	if c == "" {
		return "检验结论："
	}
	if strings.HasPrefix(c, "检验结论") {
		return c
	}
	return "检验结论：" + c
}
