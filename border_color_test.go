package pdfx

import (
	"bytes"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

// 渲染一张表，验证描边颜色（以及 alpha 时的 ExtGState）真的进了内容流。
func TestTableDrawBorderColorInStream(t *testing.T) {
	cases := []struct {
		name       string
		html       string
		wantColor  string
		wantGState bool
	}{
		{"hex8 alpha", `<table border="1pt" style="border-color:#c71919e1"><tr><td></td><td></td></tr></table>`, "0.780 0.098 0.098 RG", true},
		{"hex6", `<table border="1pt" style="border-color:#c71919"><tr><td></td><td></td></tr></table>`, "0.780 0.098 0.098 RG", false},
		{"default black", `<table border="1pt"><tr><td></td><td></td></tr></table>`, "0.000 0.000 0.000 RG", false},
	}
	for _, c := range cases {
		pdf := &gopdf.GoPdf{}
		pdf.Start(gopdf.Config{Unit: gopdf.UnitMM, PageSize: *gopdf.PageSizeA4})
		pdf.AddPage()
		pdf.SetNoCompression() // 内容流不压缩，才能直接断言里面的绘制指令
		// 表格要读当前字体，没注册字体会走到反射取字体的分支
		if err := addFont(pdf, "testfont", filepath.Join("..", "..", "assets", "fonts", "msyhl.ttc"), 0); err != nil {
			t.Skipf("skip: font unavailable: %v", err)
		}
		_ = pdf.SetFont("testfont", "", 9)
		tb := Table{X: 10, Y: 10, Width: 80, HTML: c.html}
		if err := tb.Draw(pdf); err != nil {
			t.Fatalf("%s: draw: %v", c.name, err)
		}
		out := pdf.GetBytesPdf()
		if !bytes.Contains(out, []byte(c.wantColor)) {
			t.Errorf("%s: stream has no %q", c.name, c.wantColor)
		}
		if got := bytes.Contains(out, []byte("/GS")); got != c.wantGState {
			t.Errorf("%s: /GS present = %v, want %v", c.name, got, c.wantGState)
		}
	}
}

func firstTableNode(t *testing.T, src string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var found *html.Node
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if found == nil && n.Type == html.ElementNode && strings.EqualFold(n.Data, "table") {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if found == nil {
		t.Fatalf("no <table> in %q", src)
	}
	return found
}

func TestParseHTMLBorderColor(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		color [3]uint8
		alpha float64
	}{
		{"shorthand", `<table border="1pt #333333"></table>`, [3]uint8{0x33, 0x33, 0x33}, 1},
		{"attr", `<table border="1pt" border-color="#333333"></table>`, [3]uint8{0x33, 0x33, 0x33}, 1},
		{"style", `<table border="1pt" style="width:100%; border-color:#333333"></table>`, [3]uint8{0x33, 0x33, 0x33}, 1},
		{"style shorthand", `<table style="border: 1pt solid #333333"></table>`, [3]uint8{0x33, 0x33, 0x33}, 1},
		{"hex3", `<table border="1pt" style="border-color:#333"></table>`, [3]uint8{0x33, 0x33, 0x33}, 1},
		{"rgb", `<table border="1pt" style="border-color:rgb(51,51,51)"></table>`, [3]uint8{0x33, 0x33, 0x33}, 1},
		{"hex8 alpha", `<table border="1pt" style="border-color:#c71919e1"></table>`, [3]uint8{0xc7, 0x19, 0x19}, 0xe1 / 255.0},
		{"hex4 alpha", `<table border="1pt" style="border-color:#f00c"></table>`, [3]uint8{0xff, 0x00, 0x00}, 0xcc / 255.0},
		{"rgba", `<table border="1pt" style="border-color:rgba(199,25,25,0.5)"></table>`, [3]uint8{0xc7, 0x19, 0x19}, 0.5},
	}
	for _, c := range cases {
		gotColor, gotAlpha := parseHTMLBorderColor(firstTableNode(t, c.src))
		if gotColor != c.color {
			t.Errorf("%s: color = %v, want %v", c.name, gotColor, c.color)
		}
		if math.Abs(gotAlpha-c.alpha) > 1e-9 {
			t.Errorf("%s: alpha = %v, want %v", c.name, gotAlpha, c.alpha)
		}
	}

	// 没写颜色时必须返回零值：parseBorderValue 会给裸宽度补一个黑色，
	// 若在这里漏出去就会把 border-color 顶掉（见 colorInBorderShorthand）。
	for _, src := range []string{`<table border="1pt"></table>`, `<table border="none"></table>`, `<table></table>`} {
		if c, a := parseHTMLBorderColor(firstTableNode(t, src)); c != [3]uint8{} || a != 0 {
			t.Errorf("%s: got (%v, %v), want zero", src, c, a)
		}
	}
}
