// 命令行示例：go run ./cmd/genx -o report_pdfx.pdf [-data data.json]
//
// 与 cmd/gen（pdfcore/fpdf 版）的区别：版面完全由 cmd/genx/report.tmpl 这份
// HTML 模板描述（<pdf> + <style> + <table>/<tr>/<td> + class/style），
// Go 侧只负责准备数据，不硬编码任何坐标与表格绘制。
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/w6xian/pdfx"
)

//go:embed report.tmpl
var reportTpl string

const (
	defTitle   = "出厂检验报告"
	defSig     = "reviewer_sig.png"
	defChecker = "reviewer_sig.png"
	defSeal    = "seal.png"
)

func main() {
	out := flag.String("o", "report_pdfx.pdf", "输出 PDF 路径")
	dataFile := flag.String("data", "", "JSON 数据文件；留空使用样例数据")
	assetDir := flag.String("assets", "assets", "素材目录")
	addr := flag.String("http", "", "若指定，如 :8080，则启动 HTTP 服务直接把 PDF 作为下载返回；忽略 -o")
	flag.Parse()

	d := sampleData()
	if *dataFile != "" {
		raw, err := os.ReadFile(*dataFile)
		if err != nil {
			panic(err)
		}
		if err := json.Unmarshal(raw, d); err != nil {
			panic(err)
		}
	}

	if *addr != "" {
		runHTTP(*addr, d, *assetDir)
		return
	}

	if err := Render(*out, d, *assetDir); err != nil {
		panic(err)
	}
	fmt.Println("已生成", *out)
}

// runHTTP 启动 HTTP 服务，把渲染结果直接以 application/pdf 形式回写到响应，
// 浏览器访问即可下载；演示 WithWriter / DrawHTMLBytes 两种内存输出方式
func runHTTP(addr string, d *Data, assetDir string) {
	mux := http.NewServeMux()

	// 方式一：WithWriter 直接把 PDF 写进 http.ResponseWriter
	mux.HandleFunc("/download/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="report.pdf"`)
		if err := renderTo(w, d, assetDir); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	// 方式二：DrawHTMLBytes 一次性拿到字节，自己控制响应
	mux.HandleFunc("/download/bytes", func(w http.ResponseWriter, r *http.Request) {
		pdfBytes, err := renderBytes(d, assetDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="report.pdf"`)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
		_, _ = w.Write(pdfBytes)
	})

	fmt.Printf("HTTP 已启动: http://localhost%s/download/stream  或 /download/bytes\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

// renderBytes 渲染并返回 PDF 字节
func renderBytes(d *Data, assetDir string) ([]byte, error) {
	tplHTML, song, hei, err := prepare(d, assetDir)
	if err != nil {
		return nil, err
	}
	p := pdfx.NewPDFPaper(
		pdfx.WithStyle(pdfx.A4),
		pdfx.WithFonts(
			pdfx.NewFont("songti", song),
			pdfx.NewFont("heiti", hei),
		),
	)
	return p.DrawHTMLBytes(tplHTML)
}

// renderTo 把 PDF 渲染到指定 writer（如 *bytes.Buffer 或 http.ResponseWriter）
func renderTo(w io.Writer, d *Data, assetDir string) error {
	tplHTML, song, hei, err := prepare(d, assetDir)
	if err != nil {
		return err
	}
	p := pdfx.NewPDFPaper(
		pdfx.WithStyle(pdfx.A4),
		pdfx.WithWriter(w),
		pdfx.WithFonts(
			pdfx.NewFont("songti", song),
			pdfx.NewFont("heiti", hei),
		),
	)
	return p.DrawHTML(tplHTML)
}

// prepare 准备模板字符串、字体路径
func prepare(d *Data, assetDir string) (tplHTML, song, hei string, err error) {
	if strings.TrimSpace(d.Title) == "" {
		d.Title = defTitle
	}
	v := tplData{
		Data:           *d,
		Rows:           buildRows(d.Items),
		ConclusionText: conclusionText(d),
	}
	if v.ReviewerSig == "" {
		v.ReviewerSig = assetPath(assetDir, defSig)
	}
	if v.Seal == "" {
		v.Seal = assetPath(assetDir, defSeal)
	}
	if v.Checker == "" {
		v.Checker = assetPath(assetDir, defChecker)
	}

	tpl, err := template.New("report").Funcs(template.FuncMap{
		"esc": func(s string) string { return html.EscapeString(s) },
	}).Parse(reportTpl)
	if err != nil {
		return "", "", "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, v); err != nil {
		return "", "", "", err
	}

	song, err = findFont("simsun.ttc")
	if err != nil {
		return "", "", "", err
	}
	hei, err = findFont("simhei.ttf")
	if err != nil {
		return "", "", "", err
	}
	return buf.String(), song, hei, nil
}

// Render 渲染 HTML 模板并交给 pdfx 输出 PDF
func Render(out string, d *Data, assetDir string) error {
	tplHTML, song, hei, err := prepare(d, assetDir)
	if err != nil {
		return err
	}
	p := pdfx.NewPDFPaper(
		pdfx.WithSavePath(out),
		pdfx.WithStyle(pdfx.A4),
		pdfx.WithFonts(
			pdfx.NewFont("songti", song),
			pdfx.NewFont("heiti", hei),
		),
	)
	// p.Alias("small", "col", `align="center"`, `style="font-size:9px"`) // 自定义标签别名
	return p.DrawHTML(tplHTML)
}

// findFont 在系统字体目录 / fonts / assets/fonts 中查找字体
func findFont(name string) (string, error) {
	var cands []string
	if w := os.Getenv("WINDIR"); w != "" {
		cands = append(cands, filepath.Join(w, "Fonts", name))
	}
	cands = append(cands,
		filepath.Join("C:", "Windows", "Fonts", name),
		filepath.Join("fonts", name),
		filepath.Join("assets", "fonts", name),
		filepath.Join("/usr/share/fonts/truetype/windows", name),
		filepath.Join("/System/Library/Fonts", name),
	)
	for _, p := range cands {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("genx: 未找到字体 %s", name)
}

// assetPath 素材绝对路径，不存在返回 ""
func assetPath(dir, name string) string {
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return ""
	}
	p := filepath.Join(dir, name)
	if abs, err := filepath.Abs(p); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return ""
}

// sampleData 出厂检验报告样例（实际由 RPC/JSON 传入）
func sampleData() *Data {
	return &Data{
		Company:     "重庆市永川区崔婆婆调味品有限公司",
		Title:       defTitle,
		Number:      "JC-2026-07-23-001",
		ProductName: "400gxxxxxx",
		Model:       "400g*20瓶",
		Sampler:     "黄XX",
		SamplePlace: "车间",
		SampleDate:  "2026.07.23",
		TestDate:    "2026.07.23",
		ProductDate: "2026.07.23",
		ShelfDate:   "2027.07.22",
		SampleQty:   "2瓶",
		NetWeight:   "400g",
		Basis:       "GB 2718—2014《酿造酱》",
		Items: []Item{
			{Group: "感观", GroupSpan: 3, Sub: "色泽", Standard: "具有本品固有色泽", Result: "符合", Verdict: "合格"},
			{Sub: "滋味、气味", Standard: "无异味，无异嗅", Result: "符合", Verdict: "合格"},
			{Sub: "状态", Standard: "无正常视力可见霉斑、外来异物", Result: "符合", Verdict: "合格"},
			{Name: "水分", Standard: "/", Result: "56.03", Verdict: "/"},
			{Name: "食盐", Standard: "/", Result: "6.85", Verdict: "/"},
			{Name: "酸价 ", Standard: "≥0.3", Result: "0.33", Verdict: "合格"},
			{Name: "过氧化值 ", Standard: "n=5  c=2  m=100  M=1000", Result: "<10<10<10<10<10", Verdict: "合格"},
		},
		Conclusion: "根据抽样检验符合GB 2718—2014《酿造酱》标准要求。",
		Reviewer:   "静静",
		Inspector:  "黄XX",
	}
}
