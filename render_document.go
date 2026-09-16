package pdfx

import (
	"errors"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type DocumentOptions struct {
	DefaultFontFamily string
	DefaultFontSize   float64
}

func parseHtmlToPdfNode(markup string) (*html.Node, error) {
	// 把col转成x-col避免与html的col冲突
	markup = normalizeRowCustomTags(markup)
	markup = normalizeSelfClosingTag(markup, "space")
	markup = normalizeSelfClosingTag(markup, "line")
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(markup), ctx)
	if err != nil {
		return nil, err
	}
	pdfNode := findFirstElementInNodes(nodes, "pdf")
	if pdfNode == nil {
		return nil, errors.New("pdf: <pdf> not found")
	}
	return pdfNode, nil
}

func DrawPDFFromString(pdf *gopdf.GoPdf, pdfNode *html.Node) error {
	if pdf == nil {
		return errors.New("pdf: nil GoPdf")
	}

	cssText := collectStyleText(pdfNode)
	sheet := parseSimpleStyleSheet(cssText)
	applyStyleSheetToTree(pdfNode, sheet)
	_ = setPdfFontFromNode(pdf, pdfNode)

	left, _, right, _ := pdf.Margins()
	pageW, ok := getGoPdfPageWidthInUnits(pdf)
	contentW := 0.0
	if ok && pageW > 0 {
		contentW = pageW - left - right
	}
	if contentW <= 0 {
		contentW = 100
	}

	for c := pdfNode.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if strings.EqualFold(c.Data, "style") {
			continue
		}
		switch strings.ToLower(c.Data) {
		case "row":
			_ = DrawRowFromNode(pdf, c)
		case "space":
			_ = DrawSpaceFromNode(pdf, c)
		case "line":
			_ = DrawLineFromNode(pdf, c)
		case "img":
			_ = DrawImageFromNode(pdf, c)
		case "table":
			tableHTML, err := renderOuterHTML(c)
			if err != nil {
				continue
			}
			t := Table{
				X:     left,
				Y:     pdf.GetY(),
				Width: contentW,
				HTML:  tableHTML,
			}
			_ = t.Draw(pdf)
		default:
		}
	}
	return nil
}

func DrawSpaceFromNode(pdf *gopdf.GoPdf, n *html.Node) error {
	if pdf == nil {
		return errors.New("space: nil GoPdf")
	}
	if n == nil || n.Type != html.ElementNode || !strings.EqualFold(n.Data, "space") {
		return nil
	}

	v := strings.TrimSpace(getAttr(n, "height"))
	if v == "" {
		v = strings.TrimSpace(parseStyleValue(n, "height"))
	}
	if v == "" {
		return nil
	}

	h := 0.0
	if isPureNumber(v) {
		h = parseCSSLengthToMM(v + "px")
	} else {
		h = parseCSSLengthToMM(v)
	}
	if h <= 0 {
		return nil
	}
	pdf.SetY(pdf.GetY() + h)
	return nil
}
