package pdfx

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

// PDFOption 是 PDFPaper 的配置项，配合 NewPDFPaper 使用
type PDFOption = func(p *PDFPaper)

// WithStyle 指定默认纸张类型，如 pdfx.A4
func WithStyle(style PageStyle) PDFOption {
	return func(p *PDFPaper) {
		p.style = string(style)
	}
}

// WithPadding 指定页面内边距，如 "5.0mm"
func WithPadding(padding string) PDFOption {
	return func(p *PDFPaper) {
		p.padding = padding
	}
}

// WithSavePath 指定 PDF 输出路径
func WithSavePath(savePath string) PDFOption {
	return func(p *PDFPaper) {
		p.savePath = savePath
	}
}

// WithWriter 指定 PDF 输出目标，可以是 *bytes.Buffer、http.ResponseWriter 等。
// 设置后 PDF 不落盘，WithSavePath 被忽略；适合 HTTP 直接下载。
func WithWriter(w io.Writer) PDFOption {
	return func(p *PDFPaper) {
		p.writer = w
	}
}

// Font 描述一个待注册的字体
type Font struct {
	Name            string
	Path            string
	DefaultFontSize float64
}

// NewFont 创建一个字体描述，默认字号 12
func NewFont(name, path string) Font {
	return Font{
		Name:            name,
		Path:            path,
		DefaultFontSize: 12.0,
	}
}

// WithFont 追加一个字体
func WithFont(font Font) PDFOption {
	return func(p *PDFPaper) {
		p.fonts = append(p.fonts, font)
	}
}

// WithFonts 批量设置字体，覆盖默认字体列表
func WithFonts(fonts ...Font) PDFOption {
	return func(p *PDFPaper) {
		p.fonts = append([]Font{}, fonts...)
	}
}

// PDFPaper 是一份待渲染的 HTML 文档（渲染入口）
type PDFPaper struct {
	p        *gopdf.GoPdf
	style    string
	padding  string
	savePath string
	writer   io.Writer
	fonts    []Font
	fontSize float64
	aliases  map[string]tagAlias
	vars     map[string]any
}

// tagAlias 记录一个自定义标签别名到基础标签的映射
type tagAlias struct {
	baseTag string
	attrs   []html.Attribute
}

// NewPDFPaper 创建渲染器，默认 A4 之外的 L3-1、内边距 5mm，并注册宋体/雅黑
func NewPDFPaper(opts ...PDFOption) *PDFPaper {
	p := &PDFPaper{
		style:    "L3-1",
		padding:  "5.0mm",
		savePath: "./sample.pdf",
		fonts:    []Font{NewFont("songti", "simsun.ttc"), NewFont("Yahei", "msyhbd.ttc")},
		fontSize: 12.0,
	}

	for _, opt := range opts {
		opt(p)
	}
	return p
}

// DrawHTML 渲染一段 HTML 字符串，输出到 WithWriter 指定的目标，未指定则写文件
func (p *PDFPaper) DrawHTML(html string) error {
	return p._draw_pdf(p.writer, html)
}

// DrawFile 读取 HTML 文件后渲染
func (p *PDFPaper) DrawFile(html string) error {
	htmlTableBytes, err := os.ReadFile(html)
	if err != nil {
		log.Print(err.Error())
		return err
	}
	return p._draw_pdf(p.writer, string(htmlTableBytes))
}

// DrawHTMLBytes 渲染一段 HTML 字符串并把 PDF 作为字节返回，
// 不落盘，便于 HTTP 响应直接下载；设置了 WithWriter 时该 writer 会被忽略
func (p *PDFPaper) DrawHTMLBytes(html string) ([]byte, error) {
	var buf bytes.Buffer
	if err := p._draw_pdf(&buf, html); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Alias 注册标签别名：把 aliasTag 渲染为 baseTag，并附加默认属性
func (p *PDFPaper) Alias(aliasTag string, baseTag string, attrs ...string) {
	aliasTag = strings.ToLower(strings.TrimSpace(aliasTag))
	baseTag = strings.ToLower(strings.TrimSpace(baseTag))
	if aliasTag == "" || baseTag == "" {
		return
	}
	if p.aliases == nil {
		p.aliases = make(map[string]tagAlias)
	}
	parsed := make([]html.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if k, v, ok := parseAttrSpec(a); ok {
			parsed = append(parsed, html.Attribute{Key: k, Val: v})
		}
	}
	p.aliases[aliasTag] = tagAlias{baseTag: baseTag, attrs: parsed}
}

// Var 注册模板变量，供 v-if 等表达式使用
func (p *PDFPaper) Var(name string, value any) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if p.vars == nil {
		p.vars = make(map[string]any)
	}
	p.vars[name] = value
}

func (p *PDFPaper) _draw_pdf(w io.Writer, htmlstr string) error {

	// 读取<pdf>标签中的width和height
	pdfNode, err := parseHtmlToPdfNode(htmlstr)
	if err != nil {
		log.Print(err.Error())
		return err
	}

	if len(p.aliases) > 0 {
		applyAliases(pdfNode, p.aliases)
	}
	if len(p.vars) > 0 {
		applyVIf(pdfNode, p.vars)
	}
	// <pdf paper="A4" width="214mm" height="93mm" padding="10mm 10mm 10mm 10mm">
	// pdf 页面大小 在<pdf>标签中读取width和height,padding，如果没有，width和height 请用paper中指写的

	pageW := parseCSSLengthToMM(getAttr(pdfNode, "width"))
	pageH := parseCSSLengthToMM(getAttr(pdfNode, "height"))
	paper := getAttr(pdfNode, "paper")
	pagePadding := getAttr(pdfNode, "padding")
	if pageH == 0 && paper == "" {
		return errors.New("<pdf> width, height or paper is required")
	}
	// paper 属性优先：显式指定纸张型号，如 <pdf paper="A4">
	if pageH == 0 && paper != "" {
		s := NewPager(GetPageStyle(paper))
		if s.Width > 0 && s.Height > 0 {
			pageW = s.Width
			pageH = s.Height
		}
	}
	// 未指定尺寸时回退到构造参数中的纸张型号（NewPager 返回的单位就是 mm）
	if pageH == 0 {
		s := NewPager(PageStyle(p.style))
		pageW = s.Width
		pageH = s.Height
	}
	// 页面内边距
	if pagePadding == "" {
		pagePadding = p.padding
	}

	top, right, bottom, left, err := parsePaddingShorthandToMM(pagePadding)
	if err != nil {
		log.Print(err.Error())
		return err
	}

	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{
		Unit:     gopdf.UnitMM,
		PageSize: gopdf.Rect{W: pageW, H: pageH},
	})
	pdf.SetMargins(left, top, right, bottom)
	pdf.AddPage()

	for _, font := range p.fonts {
		if err := addFont(&pdf, font.Name, font.Path, 0); err != nil {
			log.Print(err.Error())
			return err
		}
	}
	pdf.SetXY(pdf.MarginLeft(), pdf.MarginTop())

	if err := DrawPDFFromString(&pdf, pdfNode); err != nil {
		log.Print(err.Error())
		return err
	}

	// 优先写到调用方传入的 writer；否则按 WithSavePath 写文件
	if w != nil {
		if err := pdf.Write(w); err != nil {
			log.Print(err.Error())
			return err
		}
		return nil
	}
	outPath, err := filepath.Abs(p.savePath)
	if err != nil {
		outPath = p.savePath
	}
	if err := pdf.WritePdf(outPath); err != nil {
		log.Print(err.Error())
		return err
	}
	log.Print(outPath)
	return nil
}
